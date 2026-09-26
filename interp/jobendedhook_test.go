// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
)

// The hook a finished job pokes is poked for every job, and reads nothing.
//
// `Runner.JobEnded` is called on the *job's* goroutine, which is the one place
// that knows a job ended while the shell is busy with something else. That
// makes every field of the shell it might consult a field another goroutine
// writes: `JobControl` is the front end's, `monitor` moves under `set -m`, and
// the semantics vector moves under `setopt notify` — all three while jobs are
// running, and none of them synchronized against the read (#4576).
//
// So the job's goroutine asks nothing at all and the front end asks instead,
// on its own goroutine, each time round its wait — see
// Runner.NotifiesAsAJobEnds and repl/jobnotify.go, where the wake a session
// arms is what decides whether anything reaches the screen. A poke nobody is
// waiting for is dropped, which is what makes an unconditional poke free.
//
// The last two rows are the mutant: a gate restored on this side answers them
// with silence, and they say so.
func TestAFinishedJobPokesTheHookWhateverTheShellsOptionsSay(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*Runner)
	}{
		{
			"a dialect that reports a finished job at once",
			func(r *Runner) {
				r.JobControl = true
				r.Semantics.FinishedJobNoticeArrivesAtOnce = Yes
			},
		},
		{
			"a dialect that holds the notice for the next prompt",
			func(r *Runner) {
				r.JobControl = true
				r.Semantics.FinishedJobNoticeArrivesAtOnce = No
			},
		},
		{
			"a session with nobody to tell",
			func(r *Runner) {
				r.JobControl = false
				r.Semantics.FinishedJobNoticeArrivesAtOnce = Yes
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			poked := make(chan struct{}, 1)
			runLeavingJobsRunning(t, ": &", func(r *Runner) {
				tc.setup(r)
				r.JobEnded = func() {
					select {
					case poked <- struct{}{}:
					default:
					}
				}
			})
			select {
			case <-poked:
			case <-time.After(20 * time.Second):
				t.Errorf("the job ended and nothing poked Runner.JobEnded")
			}
		})
	}
}

// And the monitor moving under a live job is not a data race.
//
// The instrument is `-race` and the shape is the one #4572 caught on the hook
// itself: a job ending on its own goroutine at the same moment the shell's
// goroutine writes the option. Every iteration starts a job that finishes
// about when the `set` beside it runs, so the pair really does overlap rather
// than merely being written down next to each other.
//
// It is a race test, so it passes on a build without the detector — that is
// what `-race` in CI is for, and it is why the deterministic case above is the
// one that kills the mutant on an ordinary run.
func TestTheMonitorMovingUnderALiveJobIsNotARace(t *testing.T) {
	const src = `n=0
while [ "$n" -lt 200 ]; do
	: &
	set +m
	set -m
	n=$((n+1))
done
wait`
	deadline(t, "flipping the monitor under a live background job", func() {
		runLeavingJobsRunning(t, src, func(r *Runner) {
			// A terminal, so that `set -m` is granted rather than reaching
			// the axis that asks whether this dialect needs one — the answer
			// would be a refusal here and the loop would never move the flag.
			r.Terminal = true
			r.JobControl = true
			r.Semantics.FinishedJobNoticeArrivesAtOnce = Yes
			r.JobEnded = func() {}
		})
	})
}

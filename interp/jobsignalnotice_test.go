// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strconv"
	"strings"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A job a signal ended is named for the signal, where the dialect words one.
//
// The whole panel writes `done` for a job that simply finished and one dialect
// writes the signal's own word for one that did not: zsh 5.9.2 answers
// `kill %1` with `[1]  + terminated  sleep 30`, `kill -HUP` with `hangup`,
// `-INT` with `interrupt` and `-KILL` with `killed`. Until #4508 every one of
// those was `done` here.
//
// The words are fixed rather than taken from the machine, so what is checked
// is the shape of the notice and not what this platform calls SIGUSR1 — and
// SIGUSR1 rather than a more typical death because its default action ends the
// process without dumping core, which is the reason killednotice_test.go gives
// and the same 420K near-miss applies here.
//
// Every row runs in both states, because the half that was already right is
// the half that must not be traded away: a shell that named a signal whenever
// the status was non-zero would pass a grid that only ever killed things.
func TestAFinishedJobsNoticeNamesTheSignal(t *testing.T) {
	const killsItself = `/bin/sh -c 'kill -USR1 $$' & sleep 0.2`
	// And the same job ending by its own hand, with the same non-zero status
	// shape, which is what says the branch is keyed on the signal.
	const exitsBadly = `/bin/sh -c 'exit 7' & sleep 0.2`
	usr1 := strconv.Itoa(int(syscall.SIGUSR1))
	for _, tc := range []struct {
		name string
		src  string
		dg   Diagnostics
		want string
		// quiet says no signal word belongs in this row at all, which is
		// the half of the grid that says the wording is reached only where
		// a signal really ended the job.
		quiet bool
	}{
		{
			"the signal's word where the dialect has one",
			killsItself,
			Diagnostics{JobSignaled: "%[1]s"},
			"Boom",
			false,
		},
		{
			// The second verb, which is what bash's row is made of.
			"and its number where the dialect asks for it",
			killsItself,
			Diagnostics{JobSignaled: "%[1]s: %[2]d"},
			"Boom: " + usr1,
			false,
		},
		{
			// The reading every other column takes: no wording, so the
			// status wording stands and nothing moves for four dialects.
			"a dialect with no word for it still says what it always said",
			killsItself,
			Diagnostics{JobExited: "Exit %[1]d"},
			"Exit " + strconv.Itoa(128+int(syscall.SIGUSR1)),
			true,
		},
		{
			// Both set: the signal wins, because a killed job has a
			// non-zero status too and every shell that names a signal at
			// all names the signal.
			"the signal beats the status where both are worded",
			killsItself,
			Diagnostics{JobSignaled: "%[1]s", JobExited: "Exit %[1]d"},
			"Boom",
			false,
		},
		{
			// The pair that says what the branch is keyed on. Same
			// wordings, same non-zero status, no signal — and the signal
			// word must not appear. A branch that read the status would
			// have written `Boom` here.
			"and a job that exited badly by itself is not named for a signal",
			exitsBadly,
			Diagnostics{JobSignaled: "%[1]s", JobExited: "Exit %[1]d"},
			"Exit 7",
			true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var r *Runner
			dg := tc.dg
			dg.SignalDescriptions = map[syscall.Signal]string{syscall.SIGUSR1: "Boom"}
			notices(t, tc.src, true, No, dg, &r)
			lines := r.FinishedJobNotices()
			if len(lines) != 1 {
				t.Fatalf("notices = %v, want the finished job reported once", lines)
			}
			if !strings.Contains(lines[0], tc.want) {
				t.Errorf("notice = %q, want it to contain %q", lines[0], tc.want)
			}
			if tc.quiet && strings.Contains(lines[0], "Boom") {
				t.Errorf("notice = %q, want no signal word in it", lines[0])
			}
		})
	}
}

// And the `jobs` listing reads the same word, which is one surface rather than
// two: unlike the pid, which #4491 measured onto the notice alone, the signal
// is in the state column wherever that column is drawn.
//
// zsh cannot be asked this directly — it reports the job and forgets it in one
// motion, so no listing of a signal-killed job exists there to measure — so
// this is the core's rule and it is stated here rather than inferred from a
// shell. What it guards is the opposite mistake: a signal word wired into
// FinishedJobNotices alone would leave `jobs` calling the same job done.
func TestAListingUsesTheSignalWordToo(t *testing.T) {
	var r *Runner
	dg := Diagnostics{
		JobSignaled:        "%[1]s",
		SignalDescriptions: map[syscall.Signal]string{syscall.SIGUSR1: "Boom"},
	}
	out := notices(t, `/bin/sh -c 'kill -USR1 $$' & sleep 0.2`+"\njobs\n", true, No, dg, &r)
	if !strings.Contains(out, "Boom") {
		t.Errorf("listing = %q, want the signal's word in the state column", out)
	}
}

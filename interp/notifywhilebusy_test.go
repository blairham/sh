// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// notifyingWhileBusy makes a shell that reports a finished job the moment it
// ends: job control, the monitor, and the axis that says the notice does not
// wait for a prompt.
//
// All three, because NotifiesAsAJobEnds reads all three — a shell with nobody
// to tell arms nothing, and no shell in the panel says anything about a
// background job with the monitor off.
func notifyingWhileBusy(at Answer) func(*Runner) {
	return func(r *Runner) {
		r.JobControl = true
		r.Terminal = true
		r.SetInteractiveMonitor()
		r.Semantics.FinishedJobNoticeArrivesAtOnce = at
		// The announcement a `&` draws is a question of its own and this is
		// not it. Answered rather than left to refuse, so that the axis
		// under test is the only one this case reaches.
		r.Semantics.AnnouncesBackgroundJob = Yes
		// And the caller's own wait, which is what a foreground command is
		// run through when a front end supplies one — see
		// Runner.awaitForegroundCommand, the seam the first row is about.
		// Without it this package runs a foreground command through
		// os/exec's wait and never reaches that code at all.
		r.WaitForCommand = waitForTestCommand
	}
}

// A finished background job is reported while the shell is **busy**, and not
// only while it is idle (#4531).
//
// #4524 made the notice arrive at once where the shell is waiting for a line,
// which is the only place a front end has a descriptor in its hand. These are
// the two rows on the other side of that sentence: the shell is inside a wait
// of its own, for a foreground command or for `wait`, and the notice still
// goes in front of what that command was about to write.
//
// Measured 2026-09-25 on a pseudo-terminal against zsh 5.9.2, `-fiV +Z`, with
// `sleep 0.3 &` and no signal anywhere: `sleep 0.3 & sleep 1.5; print FGDONE`
// writes the notice a second before `FGDONE`, and `wait; print WAITED` writes
// it before `WAITED`.
//
// The `No` rows are the mutant and they are the whole point of the table: the
// option is the noun. With it off the notice is not written at all here, which
// is not a third answer but the same one — the notice waits for a prompt, and
// a script never draws one. A shell that wrote it regardless would pass both
// `Yes` rows and fail these.
func TestAFinishedJobIsReportedWhileTheShellIsBusy(t *testing.T) {
	for _, tc := range []struct {
		name   string
		src    string
		atOnce Answer
	}{
		{
			"ahead of a foreground command's output",
			"/bin/sleep 0.2 &\n/bin/sleep 1.0\nprintf MARK\n", Yes,
		},
		{
			"and not at all where the option is off",
			"/bin/sleep 0.2 &\n/bin/sleep 1.0\nprintf MARK\n", No,
		},
		{
			"before a `wait` returns",
			"/bin/sleep 0.4 &\nwait\nprintf MARK\n", Yes,
		},
		{
			"and not at all there either where the option is off",
			"/bin/sleep 0.4 &\nwait\nprintf MARK\n", No,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// One buffer for both streams, which is what makes the order
			// readable at all: the notice is written to standard error and
			// the mark to standard output, and two buffers would record each
			// of them faithfully and say nothing about which came first.
			got, _ := runLeavingJobsRunning(t, tc.src, notifyingWhileBusy(tc.atOnce))
			mark := strings.Index(got, "MARK")
			if mark < 0 {
				t.Fatalf("output = %q: the script did not reach its mark", got)
			}
			notice := strings.Index(got, "Done")
			if tc.atOnce != Yes {
				if notice >= 0 {
					t.Errorf("output = %q: a shell that holds the notice for the next prompt wrote one", got)
				}
				return
			}
			if notice < 0 {
				t.Fatalf("output = %q: no finished-job notice at all", got)
			}
			if notice > mark {
				t.Errorf("output = %q: the notice is behind the mark, want it in front", got)
			}
		})
	}
}

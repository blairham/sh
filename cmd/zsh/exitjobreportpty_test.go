// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// TestTheWayOutReportsAFinishedJobNothingReported is #6148: with the monitor
// on and NOTIFY off, a job that finished and that nothing has reported is
// reported as the shell leaves, ahead of the running-jobs sentence and the
// EXIT trap — on every route but a command string that runs off its end,
// which counts it in its warning instead (#6125). And a `wait` leaves the job
// it waited out for that report. See interp.Diagnostics.FinishedJobsReportedAtExit
// and interp.Runner.finishedJobsAwaitAJobBuiltin.
//
// Measured 2026-10-05 through a pseudo-terminal on zsh 5.9.2. The rows wait
// on what the shell writes, in order; `done` before the EXIT trap's marker is
// the order measured.
func TestTheWayOutReportsAFinishedJobNothingReported(t *testing.T) {
	const prefix = "set -m; setopt nonotify; /bin/sleep 0.1 & "
	for _, c := range []struct {
		name, body string
		last, not  []string
	}{
		{
			name: "an exit",
			body: prefix + "/bin/sleep 0.5; print -r -- x; exit",
			last: []string{"x", "[1]  + done", monitorExitEnd},
			not:  []string{"SIGHUPed"},
		},
		{
			name: "an exit beside a running job",
			body: prefix + "/bin/sleep 5 & /bin/sleep 0.5; print -r -- x; exit",
			last: []string{"x", "[1]  - done", "you have running jobs.", "warning: 1 jobs SIGHUPed"},
		},
		{
			name: "a wait leaves the job for the report",
			body: prefix + "wait; print -r -- x; exit",
			last: []string{"x", "[1]  + done", monitorExitEnd},
		},
		{
			name: "a wait leaves the job for a listing",
			body: prefix + "wait; jobs; print -r -- x",
			last: []string{"[1]  + done", "x", monitorExitEnd},
			not:  []string{"SIGHUPed"},
		},
		{
			// The control: with NOTIFY at its default the notice went out,
			// to nobody, when the job was noticed, and nothing is owed.
			name: "NOTIFY on",
			body: "set -m; /bin/sleep 0.1 & /bin/sleep 0.5; print -r -- x; exit",
			last: []string{"x", monitorExitEnd},
			not:  []string{"done"},
		},
		{
			// And the other control: the string that runs off its end
			// counts the job rather than reporting it.
			name: "the end of the string",
			body: prefix + "/bin/sleep 0.5; print -r -- x",
			last: []string{"x", monitorExitEnd, "warning: 1 jobs SIGHUPed"},
			not:  []string{"done"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			screen := monitorExitCommandString(t, c.body, c.last...)
			for _, n := range c.not {
				if strings.Contains(screen, n) {
					t.Errorf("%q should not be there; the screen was\n%s", n, smoke.Readable(smoke.LastLines(screen, 10)))
				}
			}
		})
	}
	// And a script file, which reports whether or not it ends in `exit`.
	screen := monitorExitScript(t, []string{"-f"}, prefix+"/bin/sleep 0.5\nprint -r -- x\n")
	if i, j := strings.Index(screen, "[1]  + done"), strings.Index(screen, monitorExitEnd); i < 0 || j < i {
		t.Errorf("a script's end did not report the job ahead of the EXIT trap:\n%s", smoke.Readable(smoke.LastLines(screen, 10)))
	}
}

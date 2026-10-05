// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// With the monitor on, NOTIFY at its default and no prompt, a background job
// that has finished has already left the table by the time `fg`, `bg` or
// `wait` looks for it (#6033). See interp.Runner.reportsFinishedJobsToNobody.
//
// Measured 2026-10-05 through a pseudo-terminal on zsh 5.9.2
// (/opt/homebrew/bin/zsh), as `-c` and as a script file under `-f` alike:
//
//	fg       fg: no current job, 1
//	fg %1    fg: %1: no such job, 127
//	bg       bg: no current job, 1
//	wait %1  wait: %1: no such job, 127
//
// (each one line later in the test, whose script opens with an EXIT trap),
// where this shell said bash's `job has terminated` from a table still
// holding the job. Through a pseudo-terminal because without one zsh refuses
// `set -m`.
func TestAFinishedJobHasLeftTheTableUnderANotifyingMonitor(t *testing.T) {
	for _, c := range []struct{ verb, want string }{
		{"fg", "fg:5: no current job"},
		{"fg %1", "fg:5: %1: no such job"},
		{"bg", "bg:5: no current job"},
		{"wait %1", "wait:5: %1: no such job"},
	} {
		t.Run(c.verb, func(t *testing.T) {
			body := "set -m\n/bin/sleep 0.1 &\n/bin/sleep 0.5\n" + c.verb + "\nprint -r -- rc=$?\nprint -r -- " + monitorExitFence + "\n"
			screen := monitorExitScript(t, []string{"-f"}, body)
			if strings.Contains(screen, "job has terminated") {
				t.Errorf("%s found the job:\n%s", c.verb, smoke.Readable(smoke.LastLines(screen, 10)))
			}
			if !strings.Contains(screen, c.want) {
				t.Errorf("%s: the screen was\n%s\nwant a line containing %q", c.verb, smoke.Readable(smoke.LastLines(screen, 10)), c.want)
			}
			if !strings.Contains(screen, monitorExitFence) {
				t.Errorf("the script did not reach its end:\n%s", smoke.Readable(screen))
			}
		})
	}
}

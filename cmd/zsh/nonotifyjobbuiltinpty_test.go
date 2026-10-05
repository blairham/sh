// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// With the monitor on, NOTIFY off and no prompt, a finished job stays in the
// table until a job builtin looks, and that builtin writes the job's `done`
// row before doing its own work (#6064). See
// interp.Semantics.FinishedJobIsReportedByAJobBuiltin.
//
// Measured 2026-10-05 through a pseudo-terminal on zsh 5.9.2
// (/opt/homebrew/bin/zsh) under `-f`: each verb below writes `[1]  + done
// /bin/sleep 0.1` and then its own answer, and `echo` writes nothing — the
// control, which is what says the row is the builtin's and not the job's end.
func TestAJobBuiltinReportsAFinishedJobUnderNonotify(t *testing.T) {
	const done = "[1]  + done       /bin/sleep 0.1"
	for _, c := range []struct {
		verb, after string
		reports     bool
	}{
		{"fg", "fg:6: no current job", true},
		{"jobs", "rc=0", true},
		{"wait %1", "wait:6: %1: no such job", true},
		{"echo between", "between", false},
	} {
		t.Run(c.verb, func(t *testing.T) {
			body := "set -m\nsetopt nonotify\n/bin/sleep 0.1 &\n/bin/sleep 0.5\n" + c.verb + "\nprint -r -- rc=$?\nprint -r -- " + monitorExitFence + "\n"
			screen := monitorExitScript(t, []string{"-f"}, body)
			got := strings.Contains(screen, done)
			if got != c.reports {
				t.Errorf("%s: done row written = %v, want %v; the screen was\n%s", c.verb, got, c.reports, smoke.Readable(smoke.LastLines(screen, 10)))
			}
			if c.reports && strings.Index(screen, done) > strings.Index(screen, c.after) {
				t.Errorf("%s: the done row came after %q:\n%s", c.verb, c.after, smoke.Readable(smoke.LastLines(screen, 10)))
			}
			if !strings.Contains(screen, c.after) || strings.Contains(screen, "job has terminated") {
				t.Errorf("%s: the screen was\n%s\nwant a line containing %q", c.verb, smoke.Readable(smoke.LastLines(screen, 10)), c.after)
			}
			if !strings.Contains(screen, monitorExitFence) {
				t.Errorf("the script did not reach its end:\n%s", smoke.Readable(screen))
			}
		})
	}
}

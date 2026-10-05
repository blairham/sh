// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// `fg` resumes a job started before `set -m`, under a terminal, and waits it
// out; bash, ksh93 and dash refuse it (#5927). See
// interp.Semantics.JobStartedWithoutTheMonitorIsRefused.
//
// Measured 2026-10-05 through a pseudo-terminal on zsh 5.9.2
// (/opt/homebrew/bin/zsh), as `-c` and as a script file under `-f` alike:
//
//	sleep 1 & set -m; sleep 0.3; fg       [1]  + running    sleep 1, then 0
//	sleep 1 & set -m; sleep 0.3; bg       bg: job already in background, 1
//
// This shell refused both with bash's `job 1 started without job control`.
// Through a pseudo-terminal because without one zsh refuses `set -m` and the
// state cannot be reached.
func TestFgResumesAJobStartedBeforeSetM(t *testing.T) {
	for _, c := range []struct{ verb, want string }{
		{"fg", "rc=0"},
		{"bg", "job already in background"},
	} {
		t.Run(c.verb, func(t *testing.T) {
			body := "/bin/sleep 1 &\nset -m\n/bin/sleep 0.3\n" + c.verb + "\nprint -r -- rc=$?\nprint -r -- " + monitorExitFence + "\n"
			screen := monitorExitScript(t, []string{"-f"}, body)
			if strings.Contains(screen, "without job control") {
				t.Errorf("%s refused the job:\n%s", c.verb, smoke.Readable(smoke.LastLines(screen, 10)))
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

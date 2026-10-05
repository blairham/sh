// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// TestAJobsListingWaivesTheSentenceWithNoPrompt is #6173: with no prompt, a
// `jobs` listing waives `you have running jobs.` at an `exit` in the same
// chunk or the next one, and a command string is one chunk however many
// lines it has. The exit is then not held, so it leaves with its own status
// and runs the EXIT trap behind the hangup warning. A listing in a subshell
// does not count, and in a script the waiver wears off two lines on.
//
// Measured 2026-10-05 through a pseudo-terminal on zsh 5.9.2
// (/opt/homebrew/bin/zsh), with the helper's EXIT trap on line 1 as here.
func TestAJobsListingWaivesTheSentenceWithNoPrompt(t *testing.T) {
	for _, c := range []struct {
		name, body string
		script     bool
		last       []string
		want       int
		sentence   bool
	}{
		{
			name: "a listing before the exit",
			body: "set -m; /bin/sleep 5 & jobs >/dev/null; exit 7",
			last: []string{"zsh:2: warning: 1 jobs SIGHUPed", monitorExitEnd},
			want: 7,
		},
		{
			name: "a listing in a function",
			body: "set -m; /bin/sleep 5 & f() { jobs >/dev/null }; f; exit 7",
			last: []string{"zsh:2: warning: 1 jobs SIGHUPed", monitorExitEnd},
			want: 7,
		},
		{
			name: "a listing lines before the exit of a command string",
			body: "set -m\n/bin/sleep 5 &\njobs >/dev/null\n: x\n: y\nexit 7",
			last: []string{"zsh:7: warning: 1 jobs SIGHUPed", monitorExitEnd},
			want: 7,
		},
		{
			name:     "a listing in a subshell",
			body:     "set -m\n/bin/sleep 5 &\n( jobs >/dev/null )\nexit 7",
			last:     []string{"zsh:5: you have running jobs.", "zsh:1: warning: 1 jobs SIGHUPed"},
			want:     0,
			sentence: true,
		},
		{
			name:   "a listing on the exit's line of a script",
			body:   "set -m\n/bin/sleep 5 &\njobs >/dev/null; exit 7\n",
			script: true,
			last:   []string{"job.zsh:4: warning: 1 jobs SIGHUPed", monitorExitEnd},
			want:   7,
		},
		{
			name:     "a listing two lines before a script's exit",
			body:     "set -m\n/bin/sleep 5 &\njobs >/dev/null\n: x\n: y\nexit 7\n",
			script:   true,
			last:     []string{"job.zsh:7: you have running jobs.", "job.zsh:8: warning: 1 jobs SIGHUPed", monitorExitEnd},
			want:     1,
			sentence: true,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			var screen string
			var code int
			if c.script {
				home := scratchHome(t)
				path := filepath.Join(home, "job.zsh")
				if err := os.WriteFile(path, []byte(monitorExitTrap+c.body), 0o600); err != nil {
					t.Fatalf("writing the script: %v", err)
				}
				screen, code = monitorExitRun(t, home, []string{"zsh", "-f", path}, c.last...)
			} else {
				screen, code = monitorExitCommandStatus(t, c.body, c.last...)
			}
			if code != c.want {
				t.Errorf("left with %d, want %d; the screen was\n%s", code, c.want, smoke.Readable(smoke.LastLines(screen, 10)))
			}
			if got := strings.Contains(screen, "running jobs"); got != c.sentence {
				t.Errorf("sentence written = %v, want %v; the screen was\n%s", got, c.sentence, smoke.Readable(smoke.LastLines(screen, 10)))
			}
		})
	}
}

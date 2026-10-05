// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// TestACommandStringAccountsForItsJobsByItsOwnRule is #6035: on the `-c`
// route with the monitor on, the end of the string names no job, an `exit`
// names them where a diagnostic of its line would be, the hangup warning is
// at line 1, and a job started before `set -m` is not hung up. See
// interp.Diagnostics.JobsAtExitOnACommandString and
// interp.Semantics.HangupAtExitSkipsJobsStartedWithoutTheMonitor.
//
// Measured 2026-10-05 through a pseudo-terminal on zsh 5.9.2
// (/opt/homebrew/bin/zsh). The helper puts an EXIT trap on the string's first
// line, which moves every line number one down from the measurement.
func TestACommandStringAccountsForItsJobsByItsOwnRule(t *testing.T) {
	for _, c := range []struct {
		name, body string
		want, not  []string
		// last is what the shell writes last, waited for in order.
		last []string
	}{
		{
			name: "the end of the string",
			body: "set -m; /bin/sleep 1 & print -r -- x",
			last: []string{monitorExitEnd, "zsh:1: warning: 1 jobs SIGHUPed"},
			not:  []string{"running jobs"},
		},
		{
			name: "a job started before set -m",
			body: "/bin/sleep 1 & set -m; /bin/sleep 1 & print -r -- x",
			last: []string{monitorExitEnd, "zsh:1: warning: 1 jobs SIGHUPed"},
			not:  []string{"running jobs", "2 jobs"},
		},
		{
			name: "an exit",
			body: "set -m\n/bin/sleep 1 &\nexit",
			last: []string{"zsh:4: you have running jobs.", "zsh:1: warning: 1 jobs SIGHUPed"},
			// And no EXIT trap behind the sentence: the shell is waited out
			// and the marker must not be there.
			not: []string{monitorExitEnd},
		},
		{
			name: "an exit inside a function",
			body: "set -m\n/bin/sleep 1 &\nf() {\n:\nexit\n}\nf",
			last: []string{"f:2: you have running jobs.", "zsh:1: warning: 1 jobs SIGHUPed"},
			not:  []string{monitorExitEnd},
		},
		// A job that finished and was never reported is still in the
		// table, and the warning counts it (#6125). With NOTIFY at its
		// default the report has already gone to nobody, so it is not
		// counted. Measured 2026-10-05 against zsh 5.9.2 through a pty.
		{
			name: "a finished job nothing reported",
			body: "set -m; setopt nonotify; /bin/sleep 0.1 & /bin/sleep 0.5; print -r -- x",
			last: []string{monitorExitEnd, "zsh:1: warning: 1 jobs SIGHUPed"},
		},
		{
			name: "two finished and one running",
			body: "set -m; setopt nonotify; /bin/sleep 0.1 & /bin/sleep 0.1 & /bin/sleep 5 & /bin/sleep 0.5; print -r -- x",
			last: []string{monitorExitEnd, "zsh:1: warning: 3 jobs SIGHUPed"},
		},
		{
			name: "a finished job reported to nobody",
			body: "set -m; /bin/sleep 0.1 & /bin/sleep 5 & /bin/sleep 0.5; print -r -- x",
			last: []string{monitorExitEnd, "zsh:1: warning: 1 jobs SIGHUPed"},
			not:  []string{"2 jobs"},
		},
		{
			name: "a stopped job at the end",
			// The sentence is the only thing asserted absent here: it would
			// come before the trap, and a hangup would come after it, where
			// an absence has nothing to wait for.
			body: "set -m; /bin/sleep 5 & kill -STOP $!; print -r -- x",
			last: []string{monitorExitEnd},
			not:  []string{"suspended jobs"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			screen := monitorExitCommandString(t, c.body, c.last...)
			for _, w := range c.want {
				if !strings.Contains(screen, w) {
					t.Errorf("want %q; the screen was\n%s", w, smoke.Readable(smoke.LastLines(screen, 10)))
				}
			}
			for _, n := range c.not {
				if strings.Contains(screen, n) {
					t.Errorf("%q should not be there; the screen was\n%s", n, smoke.Readable(smoke.LastLines(screen, 10)))
				}
			}
		})
	}
}

// And the status an `exit` leaves with on this route, which is neither the
// operand nor the script route's 1: measured 2026-10-05, `zsh -c 'set -m;
// sleep 1 & exit 7'` writes the sentence and leaves with 0, and with `setopt
// nocheckjobs` in front leaves with 7 — writing the warning on the `exit`'s
// own line, ahead of the EXIT trap.
func TestACommandStringsHeldExitLeavesWithZero(t *testing.T) {
	_, code := monitorExitCommandStatus(t, "set -m; /bin/sleep 1 & exit 7",
		"zsh:2: you have running jobs.", "zsh:1: warning: 1 jobs SIGHUPed")
	if code != 0 {
		t.Errorf("the held exit left with %d, want 0", code)
	}
	screen, code := monitorExitCommandStatus(t, "set -m\n/bin/sleep 1 &\nsetopt nocheckjobs\nexit 7",
		"zsh:5: warning: 1 jobs SIGHUPed", monitorExitEnd)
	if code != 7 {
		t.Errorf("an exit that wrote no sentence left with %d, want 7", code)
	}
	if strings.Contains(screen, "running jobs") {
		t.Errorf("no sentence was asked for:\n%s", smoke.Readable(screen))
	}
}

// TestACountedFinishedJobLeavesTheCommandStringWithOne is #6170: where a
// command string runs off its end and the hangup warning counts a finished
// job, the shell leaves with 1 rather than the last command's status. A count
// of running jobs alone leaves the status as it was, and so does a finished
// job nothing counts.
//
// Measured 2026-10-05 through a pseudo-terminal on zsh 5.9.2
// (/opt/homebrew/bin/zsh), each row ending in a function that returns 5 so
// that 1 and the last status cannot be confused.
func TestACountedFinishedJobLeavesTheCommandStringWithOne(t *testing.T) {
	const f = "f() { return 5 }; "
	for _, c := range []struct {
		name, body string
		last       []string
		want       int
	}{
		{
			name: "a finished job counted",
			body: f + "set -m; setopt nonotify; /bin/sleep 0.1 & /bin/sleep 0.5; f",
			last: []string{monitorExitEnd, "zsh:1: warning: 1 jobs SIGHUPed"},
			want: 1,
		},
		{
			name: "a finished job beside a running one",
			body: f + "set -m; setopt nonotify; /bin/sleep 0.1 & /bin/sleep 5 & /bin/sleep 0.5; f",
			last: []string{monitorExitEnd, "zsh:1: warning: 2 jobs SIGHUPed"},
			want: 1,
		},
		{
			name: "a running job only",
			body: f + "set -m; /bin/sleep 5 & f",
			last: []string{monitorExitEnd, "zsh:1: warning: 1 jobs SIGHUPed"},
			want: 5,
		},
		{
			name: "a finished job under nohup",
			body: f + "set -m; setopt nonotify nohup; /bin/sleep 0.1 & /bin/sleep 0.5; f",
			last: []string{monitorExitEnd},
			want: 5,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			screen, code := monitorExitCommandStatus(t, c.body, c.last...)
			if code != c.want {
				t.Errorf("left with %d, want %d; the screen was\n%s", code, c.want, smoke.Readable(smoke.LastLines(screen, 10)))
			}
		})
	}
}

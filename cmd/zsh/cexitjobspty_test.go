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

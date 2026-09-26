// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package bash_test

import (
	"regexp"
	"strings"
	"testing"
)

// A foreground subshell a signal ended is reported, exactly as an external
// command is (#4649).
//
// A real shell's `( … )` *is* a child, so there is one rule here and not a
// subshell special case — which is what the `( /bin/sh -c … )` row in
// interp's TestASubshellReportsTheSignalThatEndedItOnlyOnce guards from the
// other side.
//
// Measured 2026-09-26 against `/opt/homebrew/bin/bash --norc --noprofile` —
// GNU bash 5.3.20(1)-release, `go version -m` on it says it is not a Go
// executable — with `( kill -NAME $BASHPID ); echo "st=$?"` per row:
//
//	HUP    bash: line 1: 72060 Hangup: 1                  ( … )   129
//	INT    nothing at all                                         130
//	QUIT   bash: line 1: 72066 Quit: 3                    ( … )   131
//	KILL   bash: line 1: 72078 Killed: 9                  ( … )   137
//	TERM   Terminated: 15             ( … )                       143
//	USR1   bash: line 1: 72087 User defined signal 1: 30  ( … )   158
//
// Three shapes, and all three were already in this shell for an *external*
// command: the two quiet signals, the bare form this dialect writes for
// SIGTERM alone, and the located form with the pid for everything else. What
// was missing was the subshell reaching any of them.
//
// SIGQUIT has no row below. It is 131 in the reference and 0 here, because an
// untrapped SIGQUIT is ignored in a non-interactive shell and a body that
// signals *itself* is evidently not covered by that — a separate defect from
// this one, filed as #4724 and not papered over by leaving it out of the
// table above.
func TestAForegroundSubshellKilledBySignalIsReported(t *testing.T) {
	for _, tc := range []struct {
		name, signal, state, status string
		located                     bool
	}{
		{name: "terminate is bare", signal: "TERM", state: "Terminated: 15", status: "st=143"},
		{name: "a hangup is located", signal: "HUP", state: "Hangup: 1", status: "st=129", located: true},
		{name: "a kill is located", signal: "KILL", state: "Killed: 9", status: "st=137", located: true},
		{
			name: "and a user signal is located", signal: "USR1",
			state: "User defined signal 1: 30", status: "st=158", located: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "( kill -" + tc.signal + " $BASHPID )\necho \"st=$?\"\n"
			got := runBashAnchored(t, src)
			// The state column is 27 wide with the command straight after
			// it, which is the shape TestACommandKilledBySignalIsReported
			// already pins for an external command — asserted here whole,
			// because the padding is the part a second writer of this
			// sentence would get wrong.
			want := tc.state + strings.Repeat(" ", 27-len(tc.state)) + "( kill -" + tc.signal + " $BASHPID )"
			if !strings.Contains(got, want) {
				t.Errorf("got %q, want a notice %q", got, want)
			}
			if !strings.Contains(got, tc.status) {
				t.Errorf("got %q, want %q", got, tc.status)
			}
			located := regexp.MustCompile(`bash: line 1:\s+\d+ ` + regexp.QuoteMeta(tc.state))
			if located.MatchString(got) != tc.located {
				t.Errorf("got %q, want the located form: %v", got, tc.located)
			}
		})
	}
}

// And the two ordinary deaths are still silent, here as everywhere else.
//
// ^C is how a person stops something they started; the row exists because a
// rule written as "a subshell that died of a signal is reported" would have
// announced it.
func TestAForegroundSubshellInterruptedIsNotReported(t *testing.T) {
	got := runBashAnchored(t, "( kill -INT $BASHPID )\necho \"st=$?\"\n")
	if want := "st=130\n"; got != want {
		t.Errorf("got %q, want %q and nothing else", got, want)
	}
}

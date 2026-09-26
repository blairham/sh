// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package bash_test

import (
	"regexp"
	"strconv"
	"strings"
	"syscall"
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
// **The words for the signal are the host's and are not written down here.**
// They differ by platform in two ways at once — Linux says `Hangup` where a
// BSD says `Hangup: 1`, and SIGUSR1 is 10 there and 30 here — so what is
// asserted is the shape those words sit in: which prefix the line carries,
// where the command starts, and a status computed from the signal's own
// number. TestTheStateColumnIsNineWideWithTwoSpacesAfterIt is the same
// arrangement one surface along.
//
// SIGQUIT has no row below. It is 131 in the reference and 0 here, because an
// untrapped SIGQUIT is ignored in a non-interactive shell and a body that
// signals *itself* is evidently not covered by that — a separate defect from
// this one, filed as #4724 and not papered over by leaving it out of the
// table above.
func TestAForegroundSubshellKilledBySignalIsReported(t *testing.T) {
	for _, tc := range []struct {
		name    string
		signal  string
		number  syscall.Signal
		located bool
	}{
		{name: "terminate is bare", signal: "TERM", number: syscall.SIGTERM},
		{name: "a hangup is located", signal: "HUP", number: syscall.SIGHUP, located: true},
		{name: "a kill is located", signal: "KILL", number: syscall.SIGKILL, located: true},
		{
			name: "and a user signal is located", signal: "USR1",
			number: syscall.SIGUSR1, located: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := "( kill -" + tc.signal + " $BASHPID )"
			got := runBashAnchored(t, command+"\necho \"st=$?\"\n")
			if want := "st=" + strconv.Itoa(128+int(tc.number)); !strings.Contains(got, want) {
				t.Errorf("got %q, want %q", got, want)
			}
			line := killedNoticeLine(t, got, command)
			located := regexp.MustCompile(`^bash: line 1:\s+\d+ `)
			if where := located.FindString(line); (where != "") != tc.located {
				t.Errorf("the notice is %q, want the located form: %v", line, tc.located)
			} else if at := strings.Index(line[len(where):], command); at != killedStateColumn {
				t.Errorf("the notice is %q: the command starts at %d, want %d",
					line, at, killedStateColumn)
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
	if want := "st=" + strconv.Itoa(128+int(syscall.SIGINT)) + "\n"; got != want {
		t.Errorf("got %q, want %q and nothing else", got, want)
	}
}

// killedStateColumn is how wide the state column of a killed-command notice
// is: the words for the signal, left-aligned, with the command straight after
// them and no separator.
const killedStateColumn = 27

// killedNoticeLine is the one line of the output that is the notice.
func killedNoticeLine(t *testing.T, got, command string) string {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSuffix(got, "\n"), "\n") {
		if strings.HasSuffix(line, command) && line != command {
			return line
		}
	}
	t.Fatalf("got %q: no notice naming %q", got, command)
	return ""
}

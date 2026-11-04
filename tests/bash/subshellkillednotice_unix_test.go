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

// An EXIT trap in the body changes nothing about the notice: the shell names
// the signal that ended the body, not anything the trap ran (#7388).
//
// Measured 2026-11-02 under `-c`, each shell in a process group of its own,
// against GNU bash 5.3.20(1)-release at /opt/homebrew/bin/bash:
//
//	( trap 'echo E' EXIT; kill -USR1 $BASHPID; echo after )
//	    E, then bash: line 1: PID User defined signal 1: 30  ( … )   158
//	( trap "echo E" EXIT; kill -TERM $BASHPID; echo after )
//	    E, then Terminated: 15             ( … )                     143
//
// Both are the same notice the body gets with no trap at all. This shell
// wrote `bash: line 1: PID Signal 0: 0` for both, TERM included, because the
// trap's own `echo` cleared the record of which signal produced the status,
// and the status was put back without it.
//
// So each signal runs twice, once with no trap and once with one, and the two
// notices must agree. That holds the signal fixed and changes only the trap.
// The words are the host's, as above, so the trapped notice is checked
// against the untrapped one and never against a string written here.
func TestASubshellWithAnExitTrapIsReportedForTheSignalThatEndedIt(t *testing.T) {
	for _, tc := range []struct {
		name    string
		signal  string
		number  syscall.Signal
		located bool
	}{
		{name: "terminate is still bare", signal: "TERM", number: syscall.SIGTERM},
		{
			name: "and a user signal still located", signal: "USR1",
			number: syscall.SIGUSR1, located: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			located := regexp.MustCompile(`^bash: line 1:\s+\d+ `)
			// The state column of the notice: the words for the signal,
			// with the prefix and the command taken off.
			state := func(got, command string) string {
				t.Helper()
				line := killedNoticeLine(t, got, command)
				where := located.FindString(line)
				if (where != "") != tc.located {
					t.Fatalf("the notice is %q, want the located form: %v", line, tc.located)
				}
				words, _, ok := strings.Cut(line[len(where):], command)
				if !ok || len(words) != killedStateColumn {
					t.Fatalf("the notice is %q: the command does not start at %d",
						line, killedStateColumn)
				}
				return words
			}
			want := "st=" + strconv.Itoa(128+int(tc.number))
			plain := "( kill -" + tc.signal + " $BASHPID; echo after )"
			trapped := "( trap 'echo E' EXIT; kill -" + tc.signal + " $BASHPID; echo after )"
			without := runBashAnchored(t, plain+"\necho \"st=$?\"\n")
			with := runBashAnchored(t, trapped+"\necho \"st=$?\"\n")
			// Standard output first, so an `after` would come before the
			// status and this would not hold.
			if !strings.HasPrefix(with, "E\n"+want+"\n") {
				t.Errorf("got %q, want the trap's E and then %q", with, want)
			}
			if got, ref := state(with, trapped), state(without, plain); got != ref {
				t.Errorf("with the EXIT trap the notice names %q, without it %q", got, ref)
			}
		})
	}
}

// And an `exit` in that trap does not save the body: the copy still dies of
// the signal, so the status is the signal's and the notice names it (#7403).
//
// Measured 2026-11-04 under `-c`, each shell in a process group of its own,
// against GNU bash 5.3.20(1)-release at /opt/homebrew/bin/bash:
//
//	( trap 'echo E; exit 5' EXIT; kill -USR1 $BASHPID; echo after )
//	    E, then bash: line 1: PID User defined signal 1: 30  ( … )   158
//	( trap 'echo E; exit 5' EXIT; kill -TERM $BASHPID; echo after )
//	    E, then Terminated: 15             ( … )                     143
//	( trap 'echo E; exit' EXIT; kill -USR1 $BASHPID; echo after )   the USR1 notice, 158
//	x=$(trap 'echo E; exit 5' EXIT; kill -USR1 $BASHPID; echo after)   158, x=E
//	( … exit 5 … ) & wait $!                                         the job's notice, 158
//
// This shell left 5 for the first, second and fourth, 5 for the job, and
// named `Signal 0: 0` in every notice it wrote: the trap's `exit` was the last
// command and cleared the signal. Each row's notice is checked against the
// same body with no trap, so the words are the host's and never written here.
func TestAnExitInASubshellsExitTrapDoesNotSaveItFromItsSignal(t *testing.T) {
	located := regexp.MustCompile(`^bash: line 1:\s+\d+ `)
	// The state column of the notice: the words for the signal, with the
	// prefix and the command taken off.
	state := func(got, command string) string {
		t.Helper()
		line := killedNoticeLine(t, got, command)
		words, _, ok := strings.Cut(line[len(located.FindString(line)):], command)
		if !ok {
			t.Fatalf("the notice is %q, which does not name %q", line, command)
		}
		return words
	}
	for _, tc := range []struct {
		name, signal, exit string
		number             syscall.Signal
	}{
		{name: "a user signal and exit 5", signal: "USR1", exit: "exit 5", number: syscall.SIGUSR1},
		{name: "terminate and exit 5", signal: "TERM", exit: "exit 5", number: syscall.SIGTERM},
		{name: "a user signal and a bare exit", signal: "USR1", exit: "exit", number: syscall.SIGUSR1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := "st=" + strconv.Itoa(128+int(tc.number))
			plain := "( kill -" + tc.signal + " $BASHPID; echo after )"
			trapped := "( trap 'echo E; " + tc.exit + "' EXIT; kill -" + tc.signal + " $BASHPID; echo after )"
			without := runBashAnchored(t, plain+"\necho \"st=$?\"\n")
			with := runBashAnchored(t, trapped+"\necho \"st=$?\"\n")
			if !strings.HasPrefix(with, "E\n"+want+"\n") {
				t.Errorf("got %q, want the trap's E and then %q", with, want)
			}
			if got, ref := state(with, trapped), state(without, plain); got != ref {
				t.Errorf("with the trap's %s the notice names %q, without a trap %q", tc.exit, got, ref)
			}
		})
	}
	t.Run("a command substitution", func(t *testing.T) {
		got := runBashAnchored(t, `x=$(trap 'echo E; exit 5' EXIT; kill -USR1 $BASHPID; echo after); echo "st=$? x=$x"`)
		if want := "st=" + strconv.Itoa(128+int(syscall.SIGUSR1)) + " x=E\n"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
	t.Run("a background job", func(t *testing.T) {
		const src = `( trap 'echo E; exit 5' EXIT; kill -USR1 $BASHPID; echo after ) & wait $!; echo "st=$?"`
		got := runBashAnchored(t, src)
		if want := "E\nst=" + strconv.Itoa(128+int(syscall.SIGUSR1)) + "\n"; !strings.HasPrefix(got, want) {
			t.Errorf("got %q, want it to start %q", got, want)
		}
	})
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

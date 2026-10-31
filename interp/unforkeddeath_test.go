// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// unforkedDeathSem is keywordDeathSem — the one dialect's shape for a death,
// whose `( … )` and `$( … )` stop where they stand for one and end at 256
// plus the signal — with the answer this file is about.
func unforkedDeathSem(waits Answer) Semantics {
	s := keywordDeathSem(Yes)
	s.SignalDeathWaitsForAnUnforkedSubshell = waits
	return s
}

// handedOverWhileWaiting runs src, with every `P` in it a program that marks
// itself running and then sleeps half a second, and hands sig over the way
// the front end's watch does — from a goroutine of its own, the moment the
// program is running — as many times as offers says. It reports how the shell
// ended and whether each offer was taken.
func handedOverWhileWaiting(
	t *testing.T, src string, sem Semantics, sig syscall.Signal, offers int,
) (got leaving, taken []bool) {
	t.Helper()
	ready := filepath.Join(t.TempDir(), "ready")
	src = strings.ReplaceAll(src, "P", `/bin/sh -c ': >"$1"; exec /bin/sleep 0.5' sh `+ready)
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var o, e bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &o, Stderr: &e, Semantics: &sem, Name: "testsh",
		DieBySignal: func(s syscall.Signal) error {
			got.died = s
			return nil
		},
	})
	r.ListenForFatalSignals = func() {}
	// The wait a front end gives the shell, which is the one a death can
	// give up.
	r.WaitForCommand = waitForTestCommand
	takes := r.TakesAFatalSignal()
	ran, offered := make(chan struct{}), make(chan []bool, 1)
	go func() {
		var answers []bool
		defer func() { offered <- answers }()
		for {
			if _, err := os.Stat(ready); err == nil {
				break
			}
			select {
			case <-ran:
				return
			case <-time.After(2 * time.Millisecond):
			}
		}
		for range offers {
			claimed, _, ok := takes(sig)
			answers = append(answers, ok)
			if ok {
				// As the watch does: it waits for the shell to take it.
				select {
				case <-claimed:
				case <-time.After(10 * time.Second):
					t.Error("the shell never took the death it was handed")
				}
			}
		}
	}()
	got.status, err = r.Run(context.Background(), f)
	close(ran)
	taken = <-offered
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	got.out, got.errs = o.String(), e.String()
	return got, taken
}

// A fatal signal from outside that lands while a `( … )` or `$( … )` the
// shell runs in its own process waits for a program is held to the end of
// it in one dialect, and ends the shell at once in the others (#7068).
//
// Measured 2026-10-31, `trap 'echo "bye $?"' EXIT; (exit 7)` and the line,
// TERM from another process while the `sleep` runs, each shell in a session
// of its own: ksh93u+ writes the first column and dies of TERM at the end of
// the subshell; bash 5.3.20 writes `bye 7` and dies at once, the parentheses
// carrying on in a child of their own. This shell wrote `bye 7` in both.
func TestADeathFromOutsideWaitsForAnUnforkedSubshellInOneDialect(t *testing.T) {
	const trap = `trap 'echo "bye $?"' EXIT; (exit 7); `
	for _, tc := range []struct {
		name, src string
		yes, no   leaving
	}{
		{"parentheses", "( P; echo in ); echo after", killedByTERM("in\nbye 0\n"), killedByTERM("bye 7\n")},
		{"their status", "( P; exit 3 ); echo after", killedByTERM("bye 3\n"), killedByTERM("bye 7\n")},
		{"a substitution", "x=$(P; exit 3); echo after", killedByTERM("bye 3\n"), killedByTERM("bye 7\n")},
		{"the last command", "( P; echo in )", killedByTERM("in\nbye 0\n"), killedByTERM("bye 7\n")},
		{"an or list", "x=$(P; exit 3) || echo or", killedByTERM("bye 3\n"), killedByTERM("bye 7\n")},
		{
			"their own trap", `( trap 'echo sub' EXIT; P; echo in ); echo after`,
			killedByTERM("in\nsub\nbye 0\n"), killedByTERM("bye 7\n"),
		},
		// The pipeline's own status where it is given up, which is bash 5.3's
		// `bye 7` and not this shell's: see #7160.
		{
			"a pipeline's last element", ": | ( P; echo in ); echo after",
			killedByTERM("in\nbye 0\n"), killedByTERM("bye 0\n"),
		},
		// Inside a call the trap reads the call killed by the signal, and a
		// `function name { … }` call ends on it, either way.
		{
			"a POSIX-form call", "f() { ( P; echo in ); echo inf; }; f; echo after",
			killedByTERM("in\nbye 271\n"), killedByTERM("bye 271\n"),
		},
		{
			"a keyword call", `function f { ( P; echo in ); echo inf; }; f; echo "after $?"`,
			leaving{out: "in\nafter 271\nbye 0\n"},
			leaving{out: "after 271\nbye 0\n"},
		},
	} {
		for _, answer := range []Answer{Yes, No} {
			t.Run(tc.name+"/"+answer.String(), func(t *testing.T) {
				want := tc.no
				if answer == Yes {
					want = tc.yes
				}
				got, taken := handedOverWhileWaiting(t, trap+tc.src, unforkedDeathSem(answer), syscall.SIGTERM, 1)
				if len(taken) != 1 || !taken[0] {
					t.Errorf("taken %v, want the one offer taken", taken)
				}
				if got != want {
					t.Errorf("ended %+v, want %+v", got, want)
				}
			})
		}
	}
}

// The rest of the rule, in the dialect that waits: what reaches the subshell
// at once, what lets the death go, and what the subshell around it does
// (#7068).
//
// Measured 2026-10-31 on ksh93u+, `trap 'echo "bye $?"' EXIT; (exit 7)` and
// the line, TERM from another process while the `sleep` runs: the outer
// parentheses end as killed the moment the inner ones are over; a later
// program or subshell lets the death go and the shell exits 0; a builtin does
// not; an element the parentheses fork runs to its end; and parentheses that
// start a background job are forked by it, so the shell dies at once.
func TestWhatASubshellWaitingOutADeathDoesWithIt(t *testing.T) {
	const trap = `trap 'echo "bye $?"' EXIT; (exit 7); `
	for _, tc := range []struct {
		name, src string
		want      leaving
	}{
		{"nested", "( ( P; echo inner ); echo outer ); echo after", killedByTERM("inner\nbye 271\n")},
		{"nested with nothing after", "( ( P; echo inner ) ); echo after", killedByTERM("inner\nbye 271\n")},
		{"a substitution inside", "( x=$(P); echo outer ); echo after", killedByTERM("bye 271\n")},
		{"a later program", "( P; echo a; /bin/sleep 0.01; echo b ); echo after", leaving{out: "a\nb\nafter\nbye 0\n"}},
		{"a later subshell", `( P; x=$(echo hi); echo "b $x" ); echo after`, leaving{out: "b hi\nafter\nbye 0\n"}},
		{"a later builtin", "( P; echo a; true; echo b ); echo after", killedByTERM("a\nb\nbye 0\n")},
		{"a forked element", "( { P; echo x; } | /bin/cat; echo in ); echo after", killedByTERM("x\nin\nbye 0\n")},
		{"a background job", "( /bin/sleep 0 & P; echo in ); echo after", killedByTERM("bye 7\n")},
		{"a trap taken away", "trap 'echo x' EXIT; trap - EXIT; ( P; echo in ); echo after", killedByTERM("in\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.src
			if !strings.HasPrefix(src, "trap") {
				src = trap + src
			}
			got, taken := handedOverWhileWaiting(t, src, unforkedDeathSem(Yes), syscall.SIGTERM, 1)
			if len(taken) != 1 || !taken[0] {
				t.Errorf("taken %v, want the one offer taken", taken)
			}
			if got != tc.want {
				t.Errorf("ended %+v, want %+v", got, tc.want)
			}
		})
	}
}

// A builtin the signal lands in ends the parentheses where it lands, as a
// command killed by it, in the dialect that waits for a program there: measured
// 2026-10-31 on ksh93u+, a busy loop or a `read` inside them is `bye 271` at
// once (#7068). `offer` is the builtin.
func TestABuiltinInAnUnforkedSubshellEndsItAsKilled(t *testing.T) {
	src := `trap 'echo "bye $?"' EXIT; (exit 7); ( :; offer; echo in ); echo after`
	got, taken := leavingBy(t, src, unforkedDeathSem(Yes), syscall.SIGTERM, true)
	if !taken {
		t.Error("the death was not taken")
	}
	if want := killedByTERM("bye 271\n"); got != want {
		t.Errorf("ended %+v, want %+v", got, want)
	}
}

// A second signal while a subshell holds the first is taken as the same, and
// the shell still waits the subshell out: measured 2026-10-31 on ksh93u+, a
// second TERM 0.4 s after the first (#7068).
func TestASecondDeathWhileASubshellHoldsTheFirstIsTheFirst(t *testing.T) {
	src := `trap 'echo "bye $?"' EXIT; (exit 7); ( P; echo in ); echo after`
	got, taken := handedOverWhileWaiting(t, src, unforkedDeathSem(Yes), syscall.SIGTERM, 2)
	if len(taken) != 2 || !taken[0] || !taken[1] {
		t.Errorf("taken %v, want both offers taken", taken)
	}
	if want := killedByTERM("in\nbye 0\n"); got != want {
		t.Errorf("ended %+v, want %+v", got, want)
	}
}

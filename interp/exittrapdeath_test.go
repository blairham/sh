// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// leaving is how a shell run by leavingBy ended: the signal it asked the
// front end to die of, or none and the status it exited with.
type leaving struct {
	out, errs string
	status    int
	died      syscall.Signal
}

// leavingBy runs src with a DieBySignal that records the signal rather than
// ending this process, and with offer — where takes is not nil — a builtin
// that hands sig over the way the front end's watch hands over a signal from
// another process, and reports whether the shell took it.
func leavingBy(t *testing.T, src string, sem Semantics, sig syscall.Signal, outside bool) (got leaving, taken bool) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var o, e bytes.Buffer
	asked := 0
	r := newTestRunner(t, &Runner{
		Stdout: &o, Stderr: &e, Semantics: &sem, Name: "testsh",
		DieBySignal: func(s syscall.Signal) error {
			got.died, asked = s, asked+1
			return nil
		},
	})
	r.ListenForFatalSignals = func() {}
	if outside {
		takes := r.TakesAFatalSignal()
		r.Register("offer", func(*Runner, context.Context, []string) int {
			_, _, taken = takes(sig)
			return 0
		})
	}
	got.status, err = r.Run(context.Background(), f)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	if asked > 1 {
		t.Errorf("asked to die %d times", asked)
	}
	got.out, got.errs = o.String(), e.String()
	return got, taken
}

// exitTrapDeathSem is killSem with the two answers this file is about.
func exitTrapDeathSem(overrides, endsTheTrap Answer) Semantics {
	s := killSem()
	s.ExitInTheExitTrapOverridesASignalDeath = overrides
	s.FatalSignalInTheExitTrapEndsOnlyTheTrap = endsTheTrap
	s.SignalDeathStatusIsTwoFiftySix = No
	s.ExitInTrapReportsEarlierStatus = Yes
	return s
}

// An `exit` that names a status, in the EXIT trap a fatal signal is running,
// ends the shell at that status in one dialect and leaves the shell to die of
// the signal in the others (#7063).
//
// Measured 2026-10-30 against `trap 'echo bye; exit 5' EXIT; kill -TERM $$`:
// ksh93 exits 5, bash 5.3, bash 3.2 and bash called `sh` die of TERM. ksh93
// the same for `exit 0` and for an `exit 5` in a function the trap calls, and
// dies of TERM for a bare `exit` and for `(exit 5)`, which is the subshell's.
func TestAnExitInTheExitTrapOfADeathNamesTheStatusInOneDialect(t *testing.T) {
	const kill = "kill -TERM $$; echo after"
	for _, tc := range []struct {
		name, src string
		yes, no   leaving
	}{
		{
			name: "exit 5", src: "trap 'echo bye; exit 5' EXIT; " + kill,
			yes: leaving{out: "bye\n", status: 5}, no: leaving{out: "bye\n", status: 5, died: syscall.SIGTERM},
		},
		{
			name: "exit 0", src: "trap 'echo bye; exit 0' EXIT; " + kill,
			yes: leaving{out: "bye\n"}, no: leaving{out: "bye\n", died: syscall.SIGTERM},
		},
		{
			name: "an exit in a function", src: "f() { exit 5; }; trap 'echo bye; f; echo no' EXIT; " + kill,
			yes: leaving{out: "bye\n", status: 5}, no: leaving{out: "bye\n", status: 5, died: syscall.SIGTERM},
		},
		{
			name: "a bare exit", src: "trap 'echo bye; exit' EXIT; " + kill,
			yes: leaving{out: "bye\n", status: 143, died: syscall.SIGTERM},
			no:  leaving{out: "bye\n", status: 143, died: syscall.SIGTERM},
		},
		{
			name: "an exit in a subshell", src: `trap 'echo bye; (exit 5); echo "in $?"' EXIT; ` + kill,
			yes: leaving{out: "bye\nin 5\n", status: 143, died: syscall.SIGTERM},
			no:  leaving{out: "bye\nin 5\n", status: 143, died: syscall.SIGTERM},
		},
	} {
		for _, answer := range []Answer{Yes, No} {
			t.Run(tc.name+"/"+answer.String(), func(t *testing.T) {
				want := tc.no
				if answer == Yes {
					want = tc.yes
				}
				got, _ := leavingBy(t, tc.src, exitTrapDeathSem(answer, No), 0, false)
				if got != want {
					t.Errorf("ended %+v, want %+v", got, want)
				}
			})
		}
	}
}

// A fatal signal that lands while the EXIT trap runs ends the shell by that
// signal in five of the six columns, and ends only the trap in ksh93, which
// then leaves the way it was already leaving: at the status, or by the first
// signal (#7064).
//
// Measured 2026-10-30, each line the trap with `exit 3` or `kill -TERM $$`
// after it: bash 5.3, bash 3.2, zsh, dash and BusyBox ash write bye and die of
// USR1; ksh93 writes bye and exits 3, or dies of TERM. With `(exit 4)` and the
// script running off its end, ksh93 exits 4. A `( … )` that aims the signal at
// `$$` is the shell's own signal once the subshell is done, in both.
func TestAFatalSignalInTheExitTrapEndsTheTrapInOneDialect(t *testing.T) {
	usr1 := 128 + int(syscall.SIGUSR1)
	const trap = "trap 'echo bye; kill -USR1 $$; echo after' EXIT; "
	for _, tc := range []struct {
		name, src string
		yes, no   leaving
	}{
		{
			name: "after an exit", src: trap + "exit 3",
			yes: leaving{out: "bye\n", status: 3}, no: leaving{out: "bye\n", status: usr1, died: syscall.SIGUSR1},
		},
		{
			name: "after the script ran out", src: trap + "(exit 4)",
			yes: leaving{out: "bye\n", status: 4}, no: leaving{out: "bye\n", status: usr1, died: syscall.SIGUSR1},
		},
		{
			name: "over a death", src: trap + "kill -TERM $$; echo no",
			yes: leaving{out: "bye\n", status: 143, died: syscall.SIGTERM},
			no:  leaving{out: "bye\n", status: usr1, died: syscall.SIGUSR1},
		},
		{
			name: "from a subshell", src: "trap 'echo bye; (kill -USR1 $$; echo sub); echo after' EXIT; exit 3",
			yes: leaving{out: "bye\nsub\n", status: 3}, no: leaving{out: "bye\nsub\n", status: usr1, died: syscall.SIGUSR1},
		},
	} {
		for _, answer := range []Answer{Yes, No} {
			t.Run(tc.name+"/"+answer.String(), func(t *testing.T) {
				want := tc.no
				if answer == Yes {
					want = tc.yes
				}
				got, _ := leavingBy(t, tc.src, exitTrapDeathSem(No, answer), 0, false)
				if got != want {
					t.Errorf("ended %+v, want %+v", got, want)
				}
			})
		}
	}
}

// The same signal from another process, handed over by the front end's watch
// while the trap runs: taken to end the trap where the dialect says a signal
// does that, and left to the watch to end the process with where it does not
// — measured as above with `/bin/kill` while the trap waits for a `sleep`.
func TestAFatalSignalFromOutsideEndsTheExitTrapInOneDialect(t *testing.T) {
	const trap = "trap 'echo bye; offer; echo after' EXIT; "
	for _, tc := range []struct {
		name, src string
		answer    Answer
		taken     bool
		want      leaving
	}{
		{
			name: "after an exit", src: trap + "exit 3", answer: Yes,
			taken: true, want: leaving{out: "bye\n", status: 3},
		},
		{
			name: "over a death", src: trap + "kill -TERM $$; echo no", answer: Yes,
			taken: true, want: leaving{out: "bye\n", status: 143, died: syscall.SIGTERM},
		},
		{
			name: "in a dialect where it ends the shell", src: trap + "exit 3", answer: No,
			want: leaving{out: "bye\nafter\n", status: 3},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, taken := leavingBy(t, tc.src, exitTrapDeathSem(No, tc.answer), syscall.SIGUSR1, true)
			if taken != tc.taken {
				t.Errorf("taken = %v, want %v", taken, tc.taken)
			}
			if got != tc.want {
				t.Errorf("ended %+v, want %+v", got, tc.want)
			}
		})
	}
}

// The EXIT trap a death is running is run by a living shell: `kill` sends,
// and a copy made inside it is not born dead.
//
// This shell held the death on the Runner while the trap ran, so `kill` —
// which stops at the first target once the shell has ended itself — sent
// nothing at all, and `trap 'kill $worker' EXIT` left the worker running; and
// every `( … )` in the trap reported itself killed by signal 0. Measured
// 2026-10-30 with `kill -TERM $$` after the trap: bash 5.3 and ksh93 both
// write `k=0` for `kill -0 $$` and `in 5` alone for `(exit 5)`.
func TestTheExitTrapOfADeathIsRunByALivingShell(t *testing.T) {
	for _, tc := range []struct{ name, trap, out string }{
		{"a kill sends", `kill -0 $$; echo "k=$?"`, "k=0\n"},
		{"a subshell is not born dead", `(exit 5); echo "in $?"`, "in 5\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "trap '" + tc.trap + "' EXIT; kill -TERM $$; echo no"
			got, _ := leavingBy(t, src, exitTrapDeathSem(No, No), 0, false)
			want := leaving{out: tc.out, status: 143, died: syscall.SIGTERM}
			if got != want {
				t.Errorf("ended %+v, want %+v", got, want)
			}
		})
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
)

// segvSem is exitTrapDeathSem with ksh93's answer to a fatal signal in the
// EXIT trap — so that a SEGV there is told apart from the signal that ends
// only the trap — and segvKills as the answer this file is about.
func segvSem(segvKills Answer) Semantics {
	s := exitTrapDeathSem(No, Yes)
	s.UntrappedSegvEndsTheShellWhereItLands = segvKills
	s.SignalHandlerSeesEarlierStatus = Yes
	return s
}

// An untrapped SEGV kills the shell where it lands in one dialect, which runs
// its EXIT trap for every other fatal signal, and runs the trap first in the
// other (#7069).
//
// Measured 2026-10-30, each shell in a process group of its own: ksh93 writes
// nothing and dies of SEGV for `kill -SEGV $$` at the top, inside a function
// and from a `( … )`, where bash 5.3 and 3.2 write `bye` and die of it; and
// `trap 'echo in; kill -SEGV $$; echo still' EXIT; exit 3` writes `in` and
// dies of SEGV in ksh93, which exits 3 for the same line with TERM. BUS runs
// the trap in both, and a SEGV with a trap of its own is caught in both.
func TestAnUntrappedSegvKillsTheShellWhereItLandsInOneDialect(t *testing.T) {
	segv := 128 + int(syscall.SIGSEGV)
	died := leaving{status: segv, died: syscall.SIGSEGV}
	diedAfter := func(out string) leaving { d := died; d.out = out; return d }
	const trap = "trap 'echo bye' EXIT; "
	for _, tc := range []struct {
		name, src string
		yes, no   leaving
	}{
		{name: "at the top", src: trap + "kill -SEGV $$; echo after", yes: died, no: diedAfter("bye\n")},
		{
			name: "in a function", src: trap + "f() { kill -SEGV $$; echo in; }; f; echo after",
			yes: died, no: diedAfter("bye\n"),
		},
		{name: "from a subshell", src: trap + "(kill -SEGV $$); echo after", yes: died, no: diedAfter("bye\n")},
		{
			// Where the trap runs first, `kill` finishes before the shell
			// dies and complains about the operand it could not reach; where
			// the SEGV kills at once, there is nobody left to complain.
			name: "with more for kill to send", src: trap + "kill -SEGV $$ 99999999; echo after",
			yes: died,
			no: leaving{
				out: "bye\n", errs: "testsh: kill: (99999999) - No such process\n",
				status: segv, died: syscall.SIGSEGV,
			},
		},
		{
			name: "in the EXIT trap", src: "trap 'echo in; kill -SEGV $$; echo still' EXIT; exit 3",
			yes: diedAfter("in\n"), no: leaving{out: "in\n", status: 3},
		},
		{
			name: "another fatal signal", src: trap + "kill -BUS $$; echo after",
			yes: leaving{out: "bye\n", status: 128 + int(syscall.SIGBUS), died: syscall.SIGBUS},
			no:  leaving{out: "bye\n", status: 128 + int(syscall.SIGBUS), died: syscall.SIGBUS},
		},
		{
			name: "a SEGV with a trap of its own",
			src:  "trap 'echo caught' SEGV; " + trap + "kill -SEGV $$; echo after",
			yes:  leaving{out: "caught\nafter\nbye\n"}, no: leaving{out: "caught\nafter\nbye\n"},
		},
	} {
		for _, answer := range []Answer{Yes, No} {
			t.Run(tc.name+"/"+answer.String(), func(t *testing.T) {
				want := tc.no
				if answer == Yes {
					want = tc.yes
				}
				got, _ := leavingBy(t, tc.src, segvSem(answer), 0, false)
				if got != want {
					t.Errorf("ended %+v, want %+v", got, want)
				}
			})
		}
	}
}

// The same signal from another process, handed over by the front end's watch:
// left to the watch to end the process with where the dialect lets a SEGV
// kill where it lands — so here, where nothing ends the process, the script
// carries on — and taken to run the EXIT trap, or to end a running one, where
// it does not. Measured as above with `/bin/kill -SEGV` from another process
// at a shell in a `sleep` and at one whose EXIT trap waits for one.
func TestAnUntrappedSegvFromOutsideIsLeftToTheWatchInOneDialect(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		answer    Answer
		taken     bool
		want      leaving
	}{
		{
			name: "at a command", src: "trap 'echo bye' EXIT; offer; echo after", answer: Yes,
			want: leaving{out: "after\nbye\n"},
		},
		{
			name: "at a command, where the trap runs first", src: "trap 'echo bye' EXIT; offer; echo after", answer: No,
			taken: true, want: leaving{out: "bye\n", status: 128 + int(syscall.SIGSEGV), died: syscall.SIGSEGV},
		},
		{
			name: "in the EXIT trap", src: "trap 'echo bye; offer; echo after' EXIT; exit 3", answer: Yes,
			want: leaving{out: "bye\nafter\n", status: 3},
		},
		{
			name: "in the EXIT trap, where it ends the trap", src: "trap 'echo bye; offer; echo after' EXIT; exit 3",
			answer: No, taken: true, want: leaving{out: "bye\n", status: 3},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, taken := leavingBy(t, tc.src, segvSem(tc.answer), syscall.SIGSEGV, true)
			if taken != tc.taken {
				t.Errorf("taken = %v, want %v", taken, tc.taken)
			}
			if got != tc.want {
				t.Errorf("ended %+v, want %+v", got, tc.want)
			}
		})
	}
}

// An unanswered axis is refused by name where the trap would run for a SEGV,
// and only there: a dialect that has not said what a SEGV does is not asked
// about one it has a trap for, or about any other signal.
func TestAnUnansweredSegvAxisIsAskedOnlyWhereTheTrapWouldRun(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		refused   bool
	}{
		{"a SEGV with an EXIT trap", "trap 'echo bye' EXIT; kill -SEGV $$", true},
		{"a SEGV in the EXIT trap", "trap 'kill -SEGV $$' EXIT; exit 3", true},
		{"another fatal signal", "trap 'echo bye' EXIT; kill -BUS $$", false},
		{"a SEGV with a trap of its own", "trap 'echo caught' SEGV; trap 'echo bye' EXIT; kill -SEGV $$", false},
		{"a SEGV with no EXIT trap", "kill -SEGV $$", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := leavingBy(t, tc.src, segvSem(Unspecified), 0, false)
			refused := got.errs != ""
			if refused != tc.refused {
				t.Errorf("refused = %v (%q), want %v", refused, got.errs, tc.refused)
			}
		})
	}
}

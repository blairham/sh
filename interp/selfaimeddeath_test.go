// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
)

// selfAimedSem is killSem with the two answers this file is about, and the
// 256-plus encoding where a case asks for it so that a status the death made
// cannot be mistaken for one a command left.
func selfAimedSem(nextCommand, functionKilled Answer) Semantics {
	s := exitTrapDeathSem(No, No)
	s.SelfAimedDeathIsTakenAtTheNextCommand = nextCommand
	s.DeathInAFunctionEndsTheCallAsKilled = functionKilled
	return s
}

// The `$?` the EXIT trap reads after the shell sent itself a fatal signal is
// what `kill` and the `!` around it left in one dialect, and the status the
// `kill` began with in the other (#7062).
//
// Measured 2026-10-30, `trap 'echo "bye $?"' EXIT; (exit 7)` and then the
// line, each shell in a process group of its own: bash 5.3, bash 3.2 and bash
// called `sh` take the death at the next command and read the first column;
// ksh93 takes it as `kill` returns and reads the second. None of them runs
// what follows the `kill`, and every one dies of TERM. This shell wrote `bye
// 143` in all of them.
func TestTheExitTrapOfASelfAimedDeathReadsWhereTheShellTookIt(t *testing.T) {
	const trap = `trap 'echo "bye $?"' EXIT; (exit 7); `
	for _, tc := range []struct {
		name, src string
		yes, no   string
	}{
		{"kill", "kill -TERM $$; echo after", "bye 0\n", "bye 7\n"},
		{"a negated kill", "! kill -TERM $$; echo after", "bye 1\n", "bye 7\n"},
		{"an and list", "kill -TERM $$ && echo and; echo after", "bye 0\n", "bye 7\n"},
		{"after false", "false; kill -TERM $$; echo after", "bye 0\n", "bye 1\n"},
		{"the last command", "kill -TERM $$", "bye 0\n", "bye 7\n"},
		// What the shell held as `kill` ran is the substitution's.
		{"a substitution in the operands", "kill -TERM $(exit 4) $$; echo after", "bye 0\n", "bye 4\n"},
		// The next command is after the `!` in the first and before it in
		// the second, so the group ending is what decides.
		{"a negated group", "! { kill -TERM $$; }; echo after", "bye 1\n", "bye 7\n"},
		{"a negated group that goes on", "! { kill -TERM $$; echo in; }; echo after", "bye 0\n", "bye 7\n"},
		// The definition is the command before the `kill` here, and leaves 0.
		{"a negated call", "f() { kill -TERM $$; }; ! f; echo after", "bye 1\n", "bye 0\n"},
		// A copy's: the shell held 7 when it made the copy, and the copy
		// ran on to 0 under this vector's SubshellRunsOnAfterSignalingTheShell.
		{"a subshell", "(kill -TERM $$); echo after", "bye 7\n", "bye 0\n"},
		{"a substitution", "x=$(kill -TERM $$); echo after", "bye 7\n", "bye 0\n"},
		{"a subshell after true", "true && (kill -TERM $$; exit 4); echo after", "bye 0\n", "bye 4\n"},
	} {
		for _, answer := range []Answer{Yes, No} {
			t.Run(tc.name+"/"+answer.String(), func(t *testing.T) {
				want := leaving{out: tc.no, status: 143, died: syscall.SIGTERM}
				if answer == Yes {
					want.out = tc.yes
				}
				got, _ := leavingBy(t, trap+tc.src, selfAimedSem(answer, No), 0, false)
				if got != want {
					t.Errorf("ended %+v, want %+v", got, want)
				}
			})
		}
	}
}

// `kill` finishes its operands when the shell has caught the signal it sent
// itself to run an EXIT trap, and complains about the one after `$$` that
// names nothing; with no EXIT trap the shell dies as it sends, and the
// complaint never comes. Measured 2026-10-30 with `kill -TERM $$ 99999999`:
// bash 5.3, bash 3.2 and ksh93 write it with the trap and not without, and
// zsh and dash, which run no EXIT trap on a death, write it in neither (#7062).
func TestKillFinishesItsOperandsWhenTheShellCaughtItsOwnSignal(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		complains bool
		want      leaving
	}{
		{
			name:      "with an EXIT trap",
			src:       `trap 'echo "bye $?"' EXIT; kill -TERM $$ 99999999; echo after`,
			complains: true,
			want:      leaving{out: "bye 0\n", status: 143, died: syscall.SIGTERM},
		},
		{
			name: "without one",
			src:  `kill -TERM $$ 99999999; echo after`,
			want: leaving{status: 143, died: syscall.SIGTERM},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// KillStatusAnySuccess, so that the status `kill` returns is not
			// itself the evidence: one target answered, and that is 0.
			sem := selfAimedSem(Yes, No)
			sem.KillStatus = KillStatusAnySuccess
			got, _ := leavingBy(t, tc.src, sem, 0, false)
			if complained := strings.Contains(got.errs, "99999999"); complained != tc.complains {
				t.Errorf("complained about the second operand: %v, want %v (%q)", complained, tc.complains, got.errs)
			}
			got.errs = ""
			if got != tc.want {
				t.Errorf("ended %+v, want %+v", got, tc.want)
			}
		})
	}
}

// Inside a function the EXIT trap of a death reads the status of a command
// killed by the signal in one dialect, however the death came, and what it
// would have read at the top in the others (#7062).
//
// Measured 2026-10-30, `trap 'echo "bye $?"' EXIT` and the line, each shell in
// a process group of its own: ksh93 writes `bye 271` for a `kill` in the
// function, two calls deep, from a `( … )` in it, and from `/bin/kill` in
// another process while the function waits for a program, and `bye 7` for the
// same `kill` at the top; bash 5.3 and 3.2 write what they write at the top.
// Taken as `kill` returns here, ksh93's way, so the call is still standing
// when the death is; `offer` is a builtin and leaves 0 behind it.
func TestADeathInAFunctionReadsAsTheCallKilledInOneDialect(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		outside   bool
		yes, no   string
	}{
		{
			name: "kill", src: "f() { (exit 7); kill -TERM $$; echo inf; }; f; echo after",
			yes: "bye 271\n", no: "bye 7\n",
		},
		{
			name: "two calls deep", src: "g() { (exit 7); kill -TERM $$; }; f() { g; }; f; echo after",
			yes: "bye 271\n", no: "bye 7\n",
		},
		{
			name: "a subshell's kill", src: "f() { (exit 7); (kill -TERM $$); echo inf; }; f; echo after",
			yes: "bye 271\n", no: "bye 0\n",
		},
		{
			name: "from outside", src: "f() { (exit 7); offer; echo inf; }; f; echo after", outside: true,
			yes: "bye 271\n", no: "bye 0\n",
		},
		{name: "at the top", src: "(exit 7); kill -TERM $$; echo after", yes: "bye 7\n", no: "bye 7\n"},
	} {
		for _, answer := range []Answer{Yes, No} {
			t.Run(tc.name+"/"+answer.String(), func(t *testing.T) {
				sem := selfAimedSem(No, answer)
				sem.SignalDeathStatusIsTwoFiftySix = Yes
				want := leaving{out: tc.no, status: 143, died: syscall.SIGTERM}
				if answer == Yes {
					want.out = tc.yes
				}
				got, _ := leavingBy(t, `trap 'echo "bye $?"' EXIT; `+tc.src, sem, syscall.SIGTERM, tc.outside)
				if got != want {
					t.Errorf("ended %+v, want %+v", got, want)
				}
			})
		}
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
)

// keywordDeathSem is selfAimedSem in the shape of the one dialect that ends a
// `function name { … }` call on a death — the death taken as `kill` returns,
// a call's trap table its own, 256 plus the signal, a subshell that stops
// once it has signaled the shell, a HUP that is a death, 0 for a death during
// a pipeline — with the answer this file is about.
func keywordDeathSem(ends Answer) Semantics {
	s := selfAimedSem(No, Yes)
	s.SignalDeathStatusIsTwoFiftySix = Yes
	s.DeathDuringAPipelineShowsTheStatusBeforeIt = No
	s.SubshellRunsOnAfterSignalingTheShell = No
	s.HangupIsAnOrderlyExit = No
	s.FunctionLocalTraps = TrapsGoBackAtTheReturnOfAKeywordFunction
	s.SignalDeathEndsAtAKeywordFunctionCall = ends
	return s
}

// killedByTERM is a shell that died of TERM having written out.
func killedByTERM(out string) leaving {
	return leaving{out: out, status: 143, died: syscall.SIGTERM}
}

// A fatal signal the shell sends itself inside a `function name { … }` call
// ends the call in one dialect, which answers 256 plus the signal and goes on,
// and the shell in the others (#7145).
//
// Measured 2026-10-31, `trap 'echo "bye $?"' EXIT` and the line, each shell in
// a session of its own: ksh93u+ writes the first column and exits 0, bash
// 5.3.20 and 3.2.57 die of TERM. The POSIX-form call dies in both, and in
// ksh93 a POSIX-form call inside a keyword one ends at the keyword one.
func TestAFatalSignalEndsAKeywordCallInOneDialect(t *testing.T) {
	const trap = `trap 'echo "bye $?"' EXIT; `
	for _, tc := range []struct {
		name, src string
		yes, no   leaving
	}{
		{
			name: "kill", src: `function f { kill -TERM $$; echo inf; }; f; echo "after $?"`,
			yes: leaving{out: "after 271\nbye 0\n"}, no: killedByTERM("bye 143\n"),
		},
		{
			name: "HUP", src: `function f { (exit 4); kill -HUP $$; echo inf; }; f; echo "after $?"`,
			yes: leaving{out: "after 257\nbye 0\n"},
			no:  leaving{out: "bye 129\n", status: 129, died: syscall.SIGHUP},
		},
		{
			name: "a subshell's kill", src: `function f { (kill -TERM $$); echo "inf $?"; }; f; echo "after $?"`,
			yes: leaving{out: "after 271\nbye 0\n"}, no: killedByTERM("bye 271\n"),
		},
		{
			name: "a POSIX-form call inside it",
			src:  `g() { kill -TERM $$; echo ing; }; function f { g; echo "inf $?"; }; f; echo "after $?"`,
			yes:  leaving{out: "after 271\nbye 0\n"}, no: killedByTERM("bye 143\n"),
		},
		{
			name: "inside a POSIX-form call",
			src:  `function g { kill -TERM $$; echo ing; }; f() { g; echo "inf $?"; }; f; echo "after $?"`,
			yes:  leaving{out: "inf 271\nafter 0\nbye 0\n"}, no: killedByTERM("bye 143\n"),
		},
		{
			name: "twice", src: `function f { kill -TERM $$; }; f; echo "one $?"; f; echo "two $?"`,
			yes: leaving{out: "one 271\ntwo 271\nbye 0\n"}, no: killedByTERM("bye 143\n"),
		},
		// The POSIX form is no boundary in either: the trap reads the call
		// killed by the signal, as DeathInAFunctionEndsTheCallAsKilled says.
		{
			name: "the POSIX form", src: `f() { kill -TERM $$; echo inf; }; f; echo "after $?"`,
			yes: killedByTERM("bye 271\n"), no: killedByTERM("bye 271\n"),
		},
		{
			name: "at the top", src: `(exit 7); kill -TERM $$; echo after`,
			yes: killedByTERM("bye 7\n"), no: killedByTERM("bye 7\n"),
		},
	} {
		for _, answer := range []Answer{Yes, No} {
			t.Run(tc.name+"/"+answer.String(), func(t *testing.T) {
				want := tc.no
				if answer == Yes {
					want = tc.yes
				}
				got, _ := leavingBy(t, trap+tc.src, keywordDeathSem(answer), 0, false)
				if got != want {
					t.Errorf("ended %+v, want %+v", got, want)
				}
			})
		}
	}
}

// What has the shell catching the signal is an EXIT trap having been set,
// wherever and whenever: with none, the signal kills the shell from inside
// the call. And INT is not one the call ends on (#7067).
//
// Measured 2026-10-31 on ksh93u+, `function f { kill -TERM $$; echo inf; };
// f; echo "after $?"` behind each prefix: `after 271` and exit 0 for a trap
// set and taken away, a trap set to the empty string and a trap set in an
// earlier call that fired at its return; killed by TERM with nothing written
// for no trap at all and for a trap on USR1. `kill -INT $$` in the body kills
// it by INT with an EXIT trap standing.
func TestWhatHasTheShellCatchingASignalForAKeywordCall(t *testing.T) {
	const body = `function f { kill -TERM $$; echo inf; }; f; echo "after $?"`
	for _, tc := range []struct {
		name, src string
		want      leaving
	}{
		{"no trap", body, killedByTERM("")},
		{"a trap taken away", `trap 'echo x' EXIT; trap - EXIT; ` + body, leaving{out: "after 271\n"}},
		{"an empty trap", `trap '' EXIT; ` + body, leaving{out: "after 271\n"}},
		{
			"a trap an earlier call fired",
			`function h { trap 'echo hexit' EXIT; }; h; ` + body,
			leaving{out: "hexit\nafter 271\n"},
		},
		{"a trap on another signal", `trap 'echo usr' USR1; ` + body, killedByTERM("")},
		{
			"INT",
			`trap 'echo bye' EXIT; function f { kill -INT $$; echo inf; }; f; echo "after $?"`,
			leaving{out: "bye\n", status: 130, died: syscall.SIGINT},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := leavingBy(t, tc.src, keywordDeathSem(Yes), 0, false)
			if got != tc.want {
				t.Errorf("ended %+v, want %+v", got, tc.want)
			}
		})
	}
}

// The call's own EXIT trap runs as the signal ends it, reading what the shell
// held where the signal landed, and the call answers the signal's status
// afterwards — unless the trap ends the shell or returns from the caller.
//
// Measured 2026-10-31 on ksh93u+, each row with TERM from another process in
// place of the `kill`: `fexit 5` then `after 271`; `gexit 271`, since the
// POSIX-form call it landed in ended at 271 on the way; `gexit 0` and an exit
// at 4 with the top level's trap reading 4; `gexit 0` and `after 6`, with the
// caller's `inf` never written.
func TestAKeywordCallsOwnTrapRunsAsASignalEndsIt(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		want      leaving
	}{
		{
			"its own trap",
			`function f { trap 'echo "fexit $?"' EXIT; (exit 5); kill -TERM $$; echo inf; }; f; echo "after $?"`,
			leaving{out: "fexit 5\nafter 271\n"},
		},
		{
			"landing in a POSIX-form call",
			`function g { trap 'echo "gexit $?"' EXIT; h; echo ing; }; h() { (exit 5); kill -TERM $$; }; g; echo "after $?"`,
			leaving{out: "gexit 271\nafter 271\n"},
		},
		{
			"an exit in it",
			`trap 'echo "bye $?"' EXIT; function g { trap 'echo "gexit $?"; exit 4' EXIT; kill -TERM $$; }; g; echo "after $?"`,
			leaving{out: "gexit 0\nbye 4\n", status: 4},
		},
		{
			"a return in it",
			`function g { trap 'echo "gexit $?"; return 6' EXIT; kill -TERM $$; }; function f { g; echo "inf $?"; }; f; echo "after $?"`,
			leaving{out: "gexit 0\nafter 6\n"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := leavingBy(t, tc.src, keywordDeathSem(Yes), 0, false)
			if got != tc.want {
				t.Errorf("ended %+v, want %+v", got, tc.want)
			}
		})
	}
}

// `kill` finishes its operands before the call ends, as it does wherever the
// shell has caught its own signal: measured 2026-10-31, ksh93u+ writes the
// complaint about the second operand and then `after 271`.
func TestKillFinishesItsOperandsBeforeAKeywordCallEnds(t *testing.T) {
	src := `trap 'echo "bye $?"' EXIT; function f { kill -TERM $$ 99999999; echo "inf $?"; }; f; echo "after $?"`
	got, _ := leavingBy(t, src, keywordDeathSem(Yes), 0, false)
	if !strings.Contains(got.errs, "99999999") {
		t.Errorf("no complaint about the second operand: %q", got.errs)
	}
	got.errs = ""
	if want := (leaving{out: "after 271\nbye 0\n"}); got != want {
		t.Errorf("ended %+v, want %+v", got, want)
	}
}

// A fatal signal from another process is handed over inside a `function name
// { … }` call with the EXIT trap set aside for it, and ends the call; the next
// one is handed over as the first was. INT is not handed over for a call
// (#7067).
//
// Measured 2026-10-31 on ksh93u+ with `/bin/kill -TERM` from another process
// while the call waits on `/bin/sleep 3`: `after 271`, `bye 0`, exit 0. This
// shell had nothing to run with the trap set aside, so the front end ended
// the process where the signal landed. `offer` is a builtin and leaves 0.
func TestAFatalSignalFromOutsideEndsAKeywordCall(t *testing.T) {
	const trap = `trap 'echo "bye $?"' EXIT; `
	for _, tc := range []struct {
		name, src string
		sig       syscall.Signal
		taken     bool
		want      leaving
	}{
		{
			name: "TERM", src: `function f { :; offer; echo inf; }; f; echo "after $?"`,
			sig: syscall.SIGTERM, taken: true, want: leaving{out: "after 271\nbye 0\n"},
		},
		{
			name: "twice", src: `function f { :; offer; echo inf; }; f; echo "one $?"; f; echo "two $?"`,
			sig: syscall.SIGTERM, taken: true, want: leaving{out: "one 271\ntwo 271\nbye 0\n"},
		},
		{
			name: "with the call's own trap",
			src:  `function f { trap 'echo "fexit $?"' EXIT; (exit 5); offer; echo inf; }; f; echo "after $?"`,
			sig:  syscall.SIGTERM, taken: true, want: leaving{out: "fexit 0\nafter 271\nbye 0\n"},
		},
		{
			name: "INT", src: `function f { :; offer; echo inf; }; f; echo "after $?"`,
			sig: syscall.SIGINT, want: leaving{out: "inf\nafter 0\nbye 0\n"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, taken := leavingBy(t, trap+tc.src, keywordDeathSem(Yes), tc.sig, true)
			if taken != tc.taken {
				t.Errorf("taken %v, want %v", taken, tc.taken)
			}
			if got != tc.want {
				t.Errorf("ended %+v, want %+v", got, tc.want)
			}
		})
	}
}

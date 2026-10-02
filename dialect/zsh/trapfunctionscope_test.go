// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A handler spelled as a `TRAP…` function is scoped the way the trap it
// stands for is: an EXIT handler always, and a signal's or a
// pseudo-condition's under `localtraps`. What comes back at the return is the
// function as well as the trap, because the two spellings are one slot.
//
// Measured 2026-10-02 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin zsh -fc`
// (#5147).
func TestATrapFunctionIsScopedWithItsTrap(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a nested TRAPEXIT goes back to the caller's",
			"fn1() { TRAPEXIT() { print EXIT1 }; fn2() { TRAPEXIT() { print EXIT2 } }; fn2; functions TRAPEXIT }; fn1; functions TRAPEXIT; print st=$?",
			"EXIT2\nTRAPEXIT () {\n\tprint EXIT1\n}\nEXIT1\nst=1\n",
		},
		{
			"an EXIT trap set by a call takes the caller's function away only for the call",
			"f() { TRAPEXIT() { print E1 }; g() { trap 'print T2' EXIT }; g; print mid }; f",
			"T2\nmid\nE1\n",
		},
		{
			"a TRAPEXIT a call defines leaves the caller's action trap",
			"f() { trap 'print T1' EXIT; g() { TRAPEXIT() { print E2 } }; g; trap }; f",
			"E2\ntrap -- 'print T1' EXIT\nT1\n",
		},
		{
			"a TRAP function goes with the trap at the return",
			"fn1() { setopt localtraps; TRAPINT() { print INT1; }; }; fn1; trap; functions TRAPINT; print st=$?",
			"st=1\n",
		},
		{
			"a TRAP function gives the caller's action back",
			"trap 'print O' INT; fn1() { setopt localtraps; TRAPINT() { print INT1; }; trap; }; fn1; trap; functions TRAPINT; print st=$?",
			"TRAPINT () {\n\tprint INT1\n}\ntrap -- 'print O' INT\nst=1\n",
		},
		{
			"an action gives the caller's TRAP function back",
			"TRAPINT() { print O; }; fn1() { setopt localtraps; trap 'print T' INT; trap; }; fn1; trap",
			"trap -- 'print T' INT\nTRAPINT () {\n\tprint O\n}\n",
		},
		{
			"a redefinition gives the caller's body back",
			"TRAPINT() { print O; }; fn1() { setopt localtraps; TRAPINT() { print N; }; }; fn1; trap",
			"TRAPINT () {\n\tprint O\n}\n",
		},
		{
			"unfunction gives the caller's function back",
			"TRAPINT() { print O; }; fn1() { setopt localtraps; unfunction TRAPINT; trap; }; fn1; trap",
			"TRAPINT () {\n\tprint O\n}\n",
		},
		{
			"a reset gives the caller's function back",
			"TRAPINT() { print O; }; fn1() { setopt localtraps; trap - INT; trap; }; fn1; trap",
			"TRAPINT () {\n\tprint O\n}\n",
		},
		{
			"a pseudo-condition's function comes back too",
			"TRAPZERR() { print Z; }; fn1() { setopt localtraps; TRAPZERR() { print N; }; false; }; fn1; false",
			"N\nZ\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZshOnPath(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
			}
		})
	}
}

// A signal aimed at a background job whose body is this shell's reaches the
// body's own traps, as it reaches a forked shell's, and not the programs the
// body has started: the `sleep` goes on to the end and the handler runs
// after it. See interp/bodyinbox.go.
//
// Measured 2026-10-02 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin zsh -fc`
// (#5147).
func TestASignalToABackgroundBodyReachesItsTraps(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a trapped signal runs the body's handler after its command",
			"{ trap 'print T' TERM; sleep 0.6; print after $?; } & sleep 0.2; kill -TERM $!; wait",
			"T\nafter 0\n",
		},
		{
			"on the body's last command",
			"{ trap 'print T' TERM; sleep 0.6; } & sleep 0.2; kill -TERM $!; wait; print st=$?",
			"T\nst=0\n",
		},
		{
			"where the parentheses are the job",
			"( trap 'print T' TERM; sleep 0.6; print after ) & sleep 0.2; kill -TERM $!; wait",
			"T\nafter\n",
		},
		{
			"named by a job spec, in a function",
			"f() { trap 'print T; return 1' TERM; sleep 0.6; print no }; f & sleep 0.2; kill -TERM %1; wait",
			"T\n",
		},
		{
			"an ignored signal reaches nothing",
			"{ trap '' TERM; sleep 0.6; print after $?; } & sleep 0.2; kill -TERM $!; wait",
			"after 0\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZshOnPath(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
			}
		})
	}
}

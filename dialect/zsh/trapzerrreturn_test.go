// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A `TRAPZERR` function that returns a status other than zero makes the
// function the failure happened in return with it, or the shell exit with it
// where there is no function. Only a `return` written in the handler does
// it. See interp.Runner.forcedByATrapFunction.
//
// Measured 2026-10-02 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin zsh -fc`
// (#5147).
func TestATrapZerrFunctionsReturnIsTheFunctions(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{"a non-zero return makes the function return", "fn1() { TRAPZERR() { print trap; return 42; }; false; print Broken; }; fn1; print Working $?", "trap\nWorking 42\n", 0},
		{"return 1 the same", "fn1() { TRAPZERR() { print trap; return 1; }; false; print Broken; }; fn1; print Working $?", "trap\nWorking 1\n", 0},
		{"a bare return of a failure the same", "fn1() { TRAPZERR() { print trap; false; return; }; false; print Broken; }; fn1; print Working $?", "trap\nWorking 1\n", 0},
		{"return 0 lets the function carry on", "fn1() { TRAPZERR() { print trap; return 0; }; false; print Broken; }; fn1; print Working $?", "trap\nBroken\nWorking 0\n", 0},
		{"a failing last command is not a return", "fn1() { TRAPZERR() { print trap; (exit 3); }; false; print Broken; }; fn1; print Working $?", "trap\nBroken\nWorking 0\n", 0},
		{"it is the failing function that returns", "fn2() { false; print B2; }; fn1() { TRAPZERR() { print trap; return 42; }; fn2; print Broken $?; }; fn1; print Working $?", "trap\nBroken 42\nWorking 0\n", 0},
		{"through a brace group", "fn1() { TRAPZERR() { print trap; return 42; }; { false; print in; }; print Broken; }; fn1; print W $?", "trap\nW 42\n", 0},
		{"an action's return is the function's own", "fn1() { trap 'print trap; return 42' ZERR; false; print Broken; }; fn1; print Working $?", "trap\nWorking 42\n", 0},
		{"at the top the shell exits with it", "trap 'print X' EXIT; TRAPZERR() { print trap; return 42; }; false; print no", "trap\nX\n", 42},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZshOnPath(t, t.TempDir(), c.src)
			if out != c.want || st != c.status {
				t.Errorf("%s\n got %q, %d\nwant %q, %d", c.src, out, st, c.want, c.status)
			}
		})
	}
}

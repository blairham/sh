// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestPosixBuiltinsTakesExecOffTheShellsOwnCommands pins `posixbuiltins`
// moving Semantics.ExecReachesTheShellsOwnCommands. Measured 2026-10-02 on
// zsh 5.9.2 under `-f` (#5155): with the option on, `exec` runs only an
// external command, whatever function or builtin has the name.
func TestPosixBuiltinsTakesExecOffTheShellsOwnCommands(t *testing.T) {
	const fn = "f(){print fn $1}; "
	cases := []struct{ src, want string }{
		{fn + "(exec f a); print st=$?", "fn a\nst=0\n"},
		{fn + "setopt posixbuiltins; (exec f a); print st=$?", "zsh:1: command not found: f\nst=127\n"},
		{"setopt posixbuiltins; (exec print hi); print st=$?", "zsh:1: command not found: print\nst=127\n"},
		{"setopt posixbuiltins; cat(){print fn}; (exec cat /dev/null; print not); print st=$?", "st=0\n"},
		{fn + "emulate sh; (exec f a); print st=$?", "zsh:1: command not found: f\nst=127\n"},
		{fn + "setopt posixbuiltins; unsetopt posixbuiltins; (exec f a)", "fn a\n"},
	}
	for _, c := range cases {
		if got, _ := runZshOnPath(t, t.TempDir(), c.src); got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}

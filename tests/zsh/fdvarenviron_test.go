// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAnFdVariableIsInTheProgramsEnvironment — a program is handed the
// environment from after its own `{name}` store, which is the Yes side of
// interp.Semantics.FdVariableReachesTheProgramsEnvironment (#6521). The name
// is still put back in the shell when the program ends.
//
// Measured 2026-10-15 with `-fc` on zsh 5.9.2:
//
//	set -a; /usr/bin/env {v}</dev/null | grep ^v=                        v=11
//	export v=keep; /usr/bin/env {v}</dev/null | grep ^v=; echo "[$v]"    v=11 [keep]
func TestAnFdVariableIsInTheProgramsEnvironment(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"set -a", "set -a; /usr/bin/env {v}</dev/null | /usr/bin/grep -c ^v=", "1\n"},
		{"an exported name", "export v=keep; /usr/bin/env {v}</dev/null | /usr/bin/grep -c ^v=keep; echo \"[$v]\"", "0\n[keep]\n"},
		{"a name nobody exports", "v=keep; /usr/bin/env {v}</dev/null | /usr/bin/grep -c ^v=", "0\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

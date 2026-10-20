// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// TestAnFdVariableIsStoredThroughTheCommandsPrefix — the Yes side of
// interp.Semantics.FdVariableStoredAfterThePrefix: the prefix is assigned
// first and the store retypes it to an integer holding the number, which the
// command sees and which persists (#6569). ksh93u+ 2012-08-01 `-c`, measured
// 2026-10-15.
func TestAnFdVariableIsStoredThroughTheCommandsPrefix(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a function", `f() { echo "in[$v]"; }; v=pre f {v}</dev/null; echo "[$v]"`, "in[10]\n[10]\n"},
		{"retyped", `f() { typeset -p v; }; v=pre f {v}</dev/null; typeset -p v`, "typeset -i v=10\ntypeset -i v=10\n"},
		{"the body's assignment is arithmetic", `f() { v=x; }; v=pre f {v}</dev/null; echo "[$v]"`, "[0]\n"},
		{"a special builtin", `v=pre : {v}</dev/null; echo "[$v]"`, "[10]\n"},
		{"eval", `v=pre eval 'echo "in[$v]"' {v}</dev/null; echo "[$v]"`, "in[10]\n[10]\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

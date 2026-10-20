// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// TestAnFdVariableIsStoredThroughTheCommandsPrefix — the Yes side of
// interp.Semantics.FdVariableStoredAfterThePrefix: a function or builtin
// sees the number a `{v}` on it stored, not its own `v=` prefix, and the
// shell is left at the number once the prefix is taken back (#6569). bash
// 5.3.20 `-c`, measured 2026-10-15.
func TestAnFdVariableIsStoredThroughTheCommandsPrefix(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a function", `f() { echo "in[$v]"; }; v=pre f {v}</dev/null; echo "[$v]"`, "in[10]\n[10]\n"},
		{"in the prefix's exported cell", `f() { typeset -p v; }; v=pre f {v}</dev/null; typeset -p v`, "declare -x v=\"10\"\ndeclare -- v=\"10\"\n"},
		{"whatever the body does", `v=old; f() { v=x; }; v=pre f {v}</dev/null; echo "[$v]"`, "[10]\n"},
		{"a special builtin", `v=pre eval 'echo "in[$v]"' {v}</dev/null; echo "[$v]"`, "in[10]\n[10]\n"},
		{"a prefix over another name", `v=old; f() { echo "in[$v]"; }; v=pre f {w}</dev/null; echo "[$v]"`, "in[pre]\n[old]\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runBashSplit(t, c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

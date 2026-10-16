// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAnFdVariableIsStoredBeforeTheCommandsPrefix — the No side of
// interp.Semantics.FdVariableStoredAfterThePrefix: the store comes first, so
// the command sees its prefix — evaluated as an integer over a fresh name the
// store retyped, kept as text over a scalar — and the shell is left at the
// number (#6569). zsh 5.9.2 `-fc`, measured 2026-10-15.
func TestAnFdVariableIsStoredBeforeTheCommandsPrefix(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a function", `f() { echo "in[$v]"; }; v=pre f {v}</dev/null; echo "[$v]"`, "in[0]\n[11]\n"},
		{"over a scalar", `v=old; f() { echo "in[$v]"; }; v=pre f {v}</dev/null; echo "[$v]"`, "in[pre]\n[11]\n"},
		{"eval", `v=pre eval 'echo "in[$v]"' {v}</dev/null; echo "[$v]"`, "in[0]\n[11]\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

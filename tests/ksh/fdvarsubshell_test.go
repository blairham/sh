// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// TestAnFdVariableOnASubshellIsTheShells — a `{name}` redirection on
// parentheses publishes its number in the shell here, which is the No side of
// interp.Semantics.FdVariableOnASubshellIsTheSubshells (#6520). The
// descriptor itself still goes back when the parentheses end, which is
// FdVariableOutlivesTheCommand's No: the next allocation gets the same number.
//
// Measured 2026-10-15 with `-c` on ksh93u+ 2012-08-01:
//
//	( echo "in[$v]" ) {v}</dev/null; echo "[$v]"; exec {x}</dev/null; echo "x=$x"
//	in[10]
//	[10]
//	x=10
func TestAnFdVariableOnASubshellIsTheShells(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"numbers", "( echo \"in[$v]\" ) {v}</dev/null; echo \"[$v]\"; exec {x}</dev/null; echo \"x=$x\"\n", "in[10]\n[10]\nx=10\n"},
		{"attributes", "typeset -i v=3; ( : ) {v}</dev/null; typeset -p v\n", "typeset -i v=10\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("got %q st=%d, want %q", out, st, c.want)
			}
		})
	}
}

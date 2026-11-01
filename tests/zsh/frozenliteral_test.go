// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// interp.Semantics.FrozenContainerTakesAnElementLiteral is no here: the
// splice `b[1]=(p)` over a frozen array is refused as the array, whatever
// shape the subscript takes, with the unfrozen control splicing. Measured
// 2026-10-27 on zsh 5.9.2, `-c` (#6833).
func TestAFrozenArrayRefusesTheSplice(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"one element", `b=(x y); readonly b; b[1]=(p); echo after`, "zsh:1: read-only variable: b\n"},
		{"a declaration's", `b=(x y); readonly b; typeset b[1]=(p); echo after`, "zsh:1: read-only variable: b\n"},
		{"a range", `b=(x y z); readonly b; b[2,3]=(p); echo after`, "zsh:1: read-only variable: b\n"},
		{"unfrozen, the control", `b=(x y); b[1]=(p); typeset -p b`, "typeset -a b=( p y )\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
			}
		})
	}
}

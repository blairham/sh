// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// TestAPrefixToAProgramIsItsText — the Yes side of
// interp.Semantics.AssignmentPrefixMakesAFreshCell at a program: the prefix is
// a fresh cell, so the program is handed the text and not what the name's
// letters would make of it, where zsh and ksh93 hand `4` (#6698). GNU bash
// 5.3.20 `-c`, measured 2026-10-15.
func TestAPrefixToAProgramIsItsText(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"integer", `typeset -i v=3; v=2+2 /usr/bin/printenv v; echo "[$v]"`, "2+2\n[3]\n"},
		{"upper", `typeset -u v; v=abc /usr/bin/printenv v`, "abc\n"},
		{"a value the letter could not read", `typeset -i v; v=1+ /usr/bin/printenv v; echo "st=$?"`, "1+\nst=0\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runBash(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// TestAPrefixAppendIsFoldedByTheLetterItDisplaces — an append is the one prefix
// whose value reads through the displaced binding's case letter: the fresh
// cell has no letter of its own, and bash hands every kind of command the
// folded join, where the plain `v=cd` in the same place is handed `cd`
// (#6698). GNU bash 5.3.20 `-c`, measured 2026-10-15.
func TestAPrefixAppendIsFoldedByTheLetterItDisplaces(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a program", `declare -u v=ab; v+=cd /usr/bin/printenv v`, "ABCD\n"},
		{"lower", `declare -l v=AB; v+=CD /usr/bin/printenv v`, "abcd\n"},
		{"eval", `declare -u v=ab; v+=cd eval 'echo "$v"'; echo "[$v]"`, "ABCD\n[AB]\n"},
		{"a function", `declare -u v=ab; f() { echo "$v"; }; v+=cd f`, "ABCD\n"},
		{"listed", `declare -u v; v+=abc typeset -p v`, "declare -x v=\"ABC\"\n"},
		{"the entry behind", `declare -u v=ab; v+=cd w=$v /usr/bin/printenv w`, "ABCD\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runBash(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

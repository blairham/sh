// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A program's subscripted prefix is read by the child that runs it: the
// subscript is evaluated there, so a step it makes is not this shell's and a
// failure costs the command and not the script. Measured 2026-10-28 on zsh
// 5.9.2 from a script file, the same answer as ksh93u+, where this shell never
// read the subscript at all and ran the program at 0 (#6771). See
// interp/prefixsubscriptinturn.go.
func TestAProgramsSubscriptedPrefixIsReadInItsChild(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"a failed subscript", "b[1/0]=v /usr/bin/true; echo after $?", "zsh:2: division by zero\nafter 1\n"},
		{"a step", "b[i++]=v /usr/bin/true; echo $i", "1\n"},
		{"a step in a key", "typeset -A a; a[$((i++))]=v /usr/bin/true; echo $i", "1\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), "b=(x y z); i=1\n"+c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

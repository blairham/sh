// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestANestedNameReferenceToAListIsRefused pins that a nesting whose inner
// is a `(P)` of a name holding more than one element is refused, unquoted
// by name and quoted as a bad substitution, and ends the shell; one element
// is a name and none is none (#5151, a chunk of D04parameter.ztst). Measured
// 2026-10-03 on zsh 5.9.2 under `-f`.
func TestANestedNameReferenceToAListIsRefused(t *testing.T) {
	const setup = "x1=abc; v=(x1 x2)\n"
	for _, tc := range []struct{ src, want string }{
		{`print ${${(P)v}}; print after`, "zsh:2: parameter name reference used with array\n"},
		{`print ${${(P)v}[1,2]}`, "zsh:2: parameter name reference used with array\n"},
		{`print ${#${(P)v}}`, "zsh:2: parameter name reference used with array\n"},
		{`x=${${(PU)v}}`, "zsh:2: parameter name reference used with array\n"},
		{`set -- x1 x2; print ${${(P)@}}`, "zsh:2: parameter name reference used with array\n"},
		{`print "${${(P)v}:-z}"`, "zsh:2: bad substitution\n"},
		{`print "${${(P)v}[@]}"`, "zsh:2: bad substitution\n"},
		{`print "${(@)${(P)v}}"`, "zsh:2: bad substitution\n"},
		{"cat <<E\n${${(P)v}}\nE\necho after", "zsh:2: bad substitution\nafter\n"},
		{`(print ${${(P)v}}); echo st=$?`, "zsh:2: parameter name reference used with array\nst=1\n"},
		// The controls: one element, none, and a reference that is not nested.
		{`print ${${(P)v[1]}[1,2]}; v=(x1); print ${${(P)v}[1,2]}`, "ab\nab\n"},
		{`v=(); print "[${${(P)v}}]"`, "[]\n"},
		{`print ${(P)v} ${(P)${v}}`, "abc abc\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

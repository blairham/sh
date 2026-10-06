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

// TestANestedNameReferenceThatIsNotOneIsRefused pins that a nesting whose
// inner `(P)` holds text no parameter expression can open with is a bad
// substitution, and ends the line, in each place the nesting is read (#6202).
// Measured 2026-10-06 on zsh 5.9.2 under `-f -c` — see
// interp's refusesANestedNonName for the whole table.
func TestANestedNameReferenceThatIsNotOneIsRefused(t *testing.T) {
	const setup = "a=xyz; x=(p q)\n"
	for _, tc := range []struct{ src, want string }{
		{`e=' '; print -r -- "[${${(P)e}}]"; print after`, "zsh:2: bad substitution\n"},
		{`e=1x; print -r -- "[${${(P)e}}]"`, "zsh:2: bad substitution\n"},
		{`e=1x; print -r -- "[${${(P)e}[1]}]"`, "zsh:2: bad substitution\n"},
		{`e='a b'; print -r -- "[${#${(P)e}}]"`, "zsh:2: bad substitution\n"},
		{`e=1x; print -r -- "[${${(P)e}:-d}]"`, "zsh:2: bad substitution\n"},
		{`e=1x; y=${${(P)e}}; print -r -- "[$y]"`, "zsh:2: bad substitution\n"},
		{`e=1x; print -r -- [${${(P)e}}]`, "zsh:2: bad substitution\n"},
		{`e='#a'; print -r -- [${${(P)e}}]`, "zsh:2: bad substitution\n"},
		{`e='a[1]x'; print -r -- [${${(P)e}}]`, "zsh:2: bad substitution\n"},
		{`e=a.b; print -r -- [${${(P)e}}]`, "zsh:2: bad substitution\n"},
		{`(e=1x; print -r -- "[${${(P)e}}]"); echo st=$?`, "zsh:2: bad substitution\nst=1\n"},
		// The controls: the empty text, a reference with a subscript, a
		// special parameter, digits, the same text not nested, and text
		// that opens with a reference and goes on with an operator — which
		// zsh reads as `${a-b}`, and which is not refused here. Its value
		// is not pinned: this shell does not read the operator yet.
		{`e=; print -r -- "[${${(P)e}}]"`, "[]\n"},
		{`e='x[1]'; print -r -- "[${${(P)e}}]"`, "[p]\n"},
		{`e='?'; print -r -- "[${${(P)e}}]"`, "[0]\n"},
		{`e=10; print -r -- "[${${(P)e}}]"`, "[]\n"},
		{`e=1x; print -r -- "[${(P)e}]"`, "[]\n"},
		{`e=a-b; print -r -- "${${(P)e}}" >/dev/null; print after`, "after\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

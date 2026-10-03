// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// TestABuiltinsEqualsOpensATildeContext pins that a declaration reached as a
// builtin, `alias` and `hash` read an argument's first unquoted `=` as an
// assignment's, so the tildes after it and after each colon expand, while the
// word is otherwise split as an argument is, and that other commands leave it
// (#5670). Through the binary, because `~root` is the user database's.
// Measured 2026-10-03 on zsh 5.9.2 under -f.
func TestABuiltinsEqualsOpensATildeContext(t *testing.T) {
	const same = "same\n"
	for _, tc := range []struct{ src, out string }{
		{`r=~root; builtin local x=~root; [[ $x == $r && $r != '~root' ]] && print same`, same},
		{`r=~root; \local x=~root; [[ $x == $r ]] && print same`, same},
		{
			`r=~root; l=local; $l x=~root:~root y=$(echo a b); [[ $x == $r:$r && $y == a ]] && typeset -p b`,
			"typeset b=''\n",
		},
		{`r=~root; noglob typeset x=a:~root; [[ $x == a:$r ]] && print same`, same},
		{`r=~root; alias a=~root; [[ $aliases[a] == $r ]] && print same`, same},
		{`r=~root; hash -d nd=~root; [[ ~nd == $r ]] && print same`, same},
		{`builtin local "x=~root" y\=~root z=a~root; print -r -- $x ${(t)y-} $z`, "~root scalar a~root\n"},
		{`print -r -- x=~root; set -- y=~root; print -r -- $1`, "x=~root\ny=~root\n"},
		// A word inside the argument is a word of its own.
		{`builtin local x=${:-y=~root}; print -r -- $x`, "y=~root\n"},
	} {
		out, errs, status := runZsh(t, "-fc", tc.src)
		if out != tc.out || errs != "" || status != 0 {
			t.Errorf("%s\n got %q %q %d\nwant %q", tc.src, out, errs, status, tc.out)
		}
	}
}

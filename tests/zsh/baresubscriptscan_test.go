// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestABareSubscriptHoldsWhatAQuotedOneHolds is the reading of an unbraced
// `$name[…]` that zsh's own D06subscript asks for (#5152): a braced
// expansion, a second flag group after a range's comma, an inner subscript's
// flag group and a backslash all stay inside the brackets, and a written
// backslash reaches the matcher with its own escape taken off. Every row
// measured 2026-10-02 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`).
func TestABareSubscriptHoldsWhatAQuotedOneHolds(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`s='ab*c'; x='*'; print $s[(i)${(q)x}*]`, "3"},
		{`s=abc; print $s[(i)${(L)x}b]`, "2"},
		{`s=abc; print $s[${(L)x}1]`, "a"},
		{`a=(x y z); print "$a[${(L):-2}]"`, "y"},
		{`s='a}b'; print $s[(i)${x:-"}"}]`, "2"},
		{`s='a[b*c]'; print -R $s[(i)${(q)s[(r)\]]}]`, "6"},
		{`s='a]b[c'; print -r -- $s[(r)\],(R)\[]`, "]b["},
		{`s='a]b[c'; print -r -- $s[(r)\],(R)c]`, "]b[c"},
		{`s='a]b[c'; print -r -- ${s[(r)\],(R)\[]}`, "]b["},
		{`s='a]b[c'; print -r -- $s[(r)b,(R)\[]`, "b["},
		{`s='a[b*c]'; print -R $s[$s[(i)\[]]`, "["},
		{`s='a[b*c]'; print -R $s[(i)$s[(r)\*]]`, "1"},
		{`a=(x y z); print $a[2,3]`, "y z"},
		// A written backslash, and one that arrives in a value.
		{`a=('a\b' ab 'a?' '\' '?'); print -R $a[(i)a\\b] $a[(i)a\\?] $a[(i)\\\\] $a[(i)\\?]`, "1 3 4 5"},
		{`a=('a\b' ab 'a?' '\' '?'); print -R ${a[(i)a\\b]} ${a[(i)\\?]}`, "1 5"},
		{`a=('a\b' ab 'a?' '\' '?'); p='a\?'; q='a\b'; print -R $a[(i)$p] $a[(i)$q]`, "3 1"},
	} {
		out, _ := runZsh(t, t.TempDir(), c.src+"\n")
		if out != c.want+"\n" {
			t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want+"\n")
		}
	}
}

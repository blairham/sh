// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAnIndirectionOfAnUndeclaredNameRefusesSomeOperators pins that a `(P)`
// on a plain name nothing declared, with a `-`, `?` or `#` and a word, is a
// bad substitution that stops the shell; with `?` and no word it is nothing
// at all, and every other spelling answers as usual (#5604). Measured
// 2026-10-03 on zsh 5.9.2 under `-f`.
func TestAnIndirectionOfAnUndeclaredNameRefusesSomeOperators(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`print ${(P)nope-x}; print after`, "zsh:1: bad substitution\n"},
		{`(print "<${(P)nope?x}>"); echo st=$?`, "zsh:1: bad substitution\nst=1\n"},
		{`(print ${(P)nope#x}); (print ${(PU)nope-x}); echo st=$?`, "zsh:1: bad substitution\nzsh:1: bad substitution\nst=1\n"},
		{"cat <<E\n${(P)nope-x}\nE\necho after", "zsh:1: bad substitution\nafter\n"},
		{`print "<${(P)nope?}>" $?`, "<> 0\n"},
		// The controls: other operators, a colon, and a name that exists.
		{`print "<${(P)nope-}${(P)nope##x}${(P)nope:-x}${(P)nope+x}>"`, "<x>\n"},
		{`nope=; typeset q; a=(); print "<${(P)nope-x}${(P)q-y}${(P)a-z}${(P)1-w}>"`, "<xyzw>\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

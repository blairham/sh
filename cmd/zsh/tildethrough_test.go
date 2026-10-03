// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// TestATildeNameRunsThroughQuotesAndExpansions pins that a tilde prefix's
// name is read through quotes, a backslash and an expansion, up to the first
// `/` (and `:` in an assignment's value), once the word has been expanded
// (#5655). Through the binary, because `~root` is the user database's.
// Measured 2026-10-03 on zsh 5.9.2 under -f, each row against `r=~root`.
func TestATildeNameRunsThroughQuotesAndExpansions(t *testing.T) {
	const same = "same\n"
	for _, tc := range []struct{ src, out, errs string }{
		{`[[ ~"root" == $r && $r != '~root' ]] && print same`, same, ""},
		{`[[ ~'root'/x == $r/x && ~\root == $r && ~ro"o"t == $r ]] && print same`, same, ""},
		{`v=root; [[ ~$v == $r && ~${v} == $r && ~$(echo root) == $r ]] && print same`, same, ""},
		{`v=ro; w=root/x; [[ ~${v}ot == $r && ~$w == $r/x && ~"root/x" == $r/x ]] && print same`, same, ""},
		{`e=; [[ ~"/x" == $HOME/x && ~$e == $HOME && ~"+" == $PWD ]] && print same`, same, ""},
		{`a=(root x); b=(~$a); [[ $#b == 2 && $b[1] == $r && $b[2] == x ]] && print same`, same, ""},
		{`x=~"root" y=a:~"root" z=~"root":~"root"; [[ $x == $r && $y == a:$r && $z == $r:$r ]] && print same`, same, ""},
		{`[[ $r == ~"root" ]] && case $r in ~"root") print same;; esac`, same, ""},
		// Not a name, or a value's own tilde: as written.
		{`v="ro ot" w=":~root"; x=a$w; print -r -- ~"a b" ~$v ~"root":x $x`, "~a b ~ro ot ~root:x a:~root\n", ""},
		{`print ~root"x"; print after`, "", "zsh:2: no such user or named directory: rootx\n"},
	} {
		out, errs, _ := runZsh(t, "-fc", "r=~root\n"+tc.src)
		if out != tc.out || errs != tc.errs {
			t.Errorf("%s\n got %q %q\nwant %q %q", tc.src, out, errs, tc.out, tc.errs)
		}
	}
}

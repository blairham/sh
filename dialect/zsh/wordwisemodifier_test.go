// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestTheWPrefixAppliesAModifierToEachWord pins the `w` modifier prefix: the
// modifier behind it applied to each `$IFS`-separated word, separators kept
// where they were (#5151, a chunk of D04parameter.ztst). Measured 2026-10-02
// on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`, `LC_ALL=C`).
func TestTheWPrefixAppliesAModifierToEachWord(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`foo=; print -r -- "<${foo:wq}>" "<${:wq}>"`, "<> <>\n"},
		{`foo="a.b  c.d"; print -r -- "<${foo:wr}>" "<${foo:wu}>"`, "<a  c> <A.B  C.D>\n"},
		{`IFS=:; foo="a.b:c.d"; print -r -- "<${foo:wr}>"`, "<a:c>\n"},
		{`foo="a.b:c.d e.f"; print -r -- "<${foo:wr}>" "<${foo:wgs/./_/}>" "<${foo:wfr}>"`, "<a.b:c e> <a_b:c_d e_f> <a e>\n"},
		{`a=(x.c "y z.h"); print -r -- ${a:wr}`, "x y z\n"},
		{`foo=' a/b '; print -r -- "<${foo:wh}>"; foo='a  b'; print -r -- "<${foo:wh}>"`, "< a .>\n<.  .>\n"},
		{`foo="a b"; print -r -- "${foo:w}"`, "zsh:1: unrecognized modifier `w'\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

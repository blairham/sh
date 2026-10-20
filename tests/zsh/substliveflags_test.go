// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestASubstitutionsLiveCharactersPassTheFlagsAndANesting pins that the
// pattern characters a `:s` replacement writes stay live through `(j)`, `(q)`,
// `(l)` and `(r)`, through a nesting and the modifiers and range around it,
// and through the `:q` modifier (#5640). Measured 2026-10-03 on zsh 5.9.2
// under -f, beside `xay` and `xby`.
func TestASubstitutionsLiveCharactersPassTheFlagsAndANesting(t *testing.T) {
	const setup = "print -n >xay; print -n >xby; s=xQy\n"
	for _, tc := range []struct{ src, want string }{
		{`print ${(j:,:)s:s/Q/?/} ${(q)s:s/Q/?/} ${(r:3:)s:s/Q/?/}`, "xay xby xay xby xay xby\n"},
		{`a=(xQy xQy); print ${(j:,:)a:s/Q/?/}`, "zsh:2: no matches found: x?y,x?y\n"},
		// A padding counts the marked character as one, and keeps it.
		{`print ${(l:5:)s:s/Q/?/}`, "zsh:2: no matches found:   x?y\n"},
		{`print ${(l:2:)s:s/Q/?/}`, "zsh:2: no matches found: ?y\n"},
		{`print ${(r:2:)s:s/Q/?/}`, "zsh:2: no matches found: x?\n"},
		{`print ${(l:4::x:)s:s/Q/?/}`, "zsh:2: no matches found: xx?y\n"},
		// The quoting styles other than the single `q` make it text.
		{`print ${(qq)s:s/Q/?/}`, "'x?y'\n"},
		{`print ${${s:s/Q/?/}} ${${${s:s/Q/?/}}} ${${s:s/Q/?/}:-z}`, "xay xby xay xby xay xby\n"},
		{`print ${(j:,:)${s:s/Q/?/}} ${(q)${s:s/Q/?/}} ${${s:s/Q/?/}:r}`, "xay xby xay xby xay xby\n"},
		{`print ${(U)${s:s/Q/?/}}`, "zsh:2: no matches found: X?Y\n"},
		{`print ${${s:s/Q/?/}:u}`, "zsh:2: no matches found: X?Y\n"},
		{`print ${(l:5:)${s:s/Q/?/}}`, "zsh:2: no matches found:   x?y\n"},
		{`a=(xQy xQb); print ${${a:s/Q/?/}}`, "zsh:2: no matches found: x?b\n"},
		{`t='x?y'; print ${${~t}}`, "xay xby\n"},
		// A marked `?` is not the text `?` a later pattern names.
		{`print ${${s:s/Q/?/}:s/x?/z/}`, "xay xby\n"},
		{`print ${${s:s/Q/?/}:1}`, "zsh:2: no matches found: ?y\n"},
		{`s=xQyy; print ${${s:s/Q/?/}:0:3}`, "xay xby\n"},
		{`print ${s:s/Q/?/:q} ${${s:s/Q/?/}:q}`, "xay xby xay xby\n"},
		// An operator that makes new text of it, or quotes, leave none.
		{`print ${${s:s/Q/?/}#x} ${${s:s/Q/?/}/y/y} ${(e)${s:s/Q/?/}}`, "?y x?y x?y\n"},
		{`print ${s:s/Q/?/:Q} "${${s:s/Q/?/}}"; v=${${s:s/Q/?/}}; print $v`, "x?y x?y\nx?y\n"},
		{`print ${${:-"l[]o"}:s/[]//} ${${:-"x[ab]y"}}`, "lo x[ab]y\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

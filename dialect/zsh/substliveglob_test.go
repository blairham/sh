// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestASubstitutionsReplacementKeepsItsPatternCharacters pins that a pattern
// character a `:s` replacement wrote unquoted stays live in the result, where
// a value's own characters are text (#5601). Measured 2026-10-03 on zsh 5.9.2
// under `-f`, in a directory holding `xay` and `xby`.
func TestASubstitutionsReplacementKeepsItsPatternCharacters(t *testing.T) {
	const setup = "print -n >xay; print -n >xby; s=xQy\n"
	for _, tc := range []struct{ src, want string }{
		{`print ${s:s/Q/?/} ${s:s/Q/[ab]/} $s:s/Q/*/`, "xay xby xay xby xay xby\n"},
		{`print ${s:gs/Q/?/} ${s:s/Q/?/:s/x/x/} ${(L)s:s/Q/?/}`, "xay xby xay xby xay xby\n"},
		{`print ${s:s/Q/?/}.`, "zsh:2: no matches found: x?y.\n"},
		{`print ${(U)s:s/Q/?/}`, "zsh:2: no matches found: X?Y\n"},
		{`s='x?Q'; print ${s:s/Q/?/}`, "zsh:2: no matches found: x??\n"},
		{`[[ xay = ${s:s/Q/?/} ]] && print match; case xby in ${s:s/Q/?/}) print arm;; esac`, "match\narm\n"},
		// Quoted, in the replacement or around the expansion, it is text.
		{`print "${s:s/Q/?/}" ${s:s/Q/\?/} ${s:s/Q/'?'/} ${s:s/Q/"?"/}`, "x?y x?y x?y x?y\n"},
		{`v=${s:s/Q/?/}; print -r -- $v ${(c)#s:s/Q/?/} ${(b)s:s/Q/?/}`, "x?y 3 x\\?y\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

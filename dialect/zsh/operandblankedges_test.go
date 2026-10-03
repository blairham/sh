// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestUnderShWordSplitAnOperandsBlanksSeparateIt pins that under
// `shwordsplit` the blanks at either end of a word a `-` or `+` substituted
// end the field in front of the expansion and open the one behind it; with
// the option off the word is one field (#5151, a chunk of D04parameter.ztst).
// Measured 2026-10-03 on zsh 5.9.2 under `-f`.
func TestUnderShWordSplitAnOperandsBlanksSeparateIt(t *testing.T) {
	const f = "f(){ printf '<%s>' \"$@\"; echo; }\n"
	for _, tc := range []struct{ src, want string }{
		{`setopt shwordsplit; f x${:- p }y x${:- p q }y`, "<x><p><y><x><p><q><y>\n"},
		{`setopt shwordsplit; f x${:-p }y x${:- p}y x${:- }y`, "<xp><y><x><py><x><y>\n"},
		{`setopt shwordsplit; v=1; f x${v:+ p }y`, "<x><p><y>\n"},
		{`setopt shwordsplit; IFS=:; f x${u:-p:}y`, "<xp><y>\n"},
		{`f x${:- p }y`, "<x p y>\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), f+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAWordSubscriptCountsWords pins `(w)`, its `(s:…:)` separator and the
// `(p)` that reads escapes in that separator: the lines reading `(f)` already
// has, over another separator (#5152). Measured 2026-10-02 on zsh 5.9.2
// (`/opt/homebrew/bin/zsh -f`, `LC_ALL=C`).
func TestAWordSubscriptCountsWords(t *testing.T) {
	const setup = "t=$'a\\tb c'; u=axybxyc\n"
	for _, tc := range []struct{ src, want string }{
		{`print -r -- "<${t[(w)2]}><${t[(ws: :)2]}><${t[(pws:\t:)2]}><${t[(wps:\t:)2]}>"`, "<b><c><b c><b c>\n"},
		// A `p` behind the `s` reads nothing, so the separator is two
		// characters found nowhere and the second word clamps to the one.
		{`print -r -- "<${t[(ws:\t:p)2]}>"`, "<a\tb c>\n"},
		{`IFS=:; print -r -- "<${t[(w)1]}>"`, "<a\tb c>\n"},
		{`s=$'foo\0bar'; print -r -- ${s[(pws:\0:)1]} ${s[(pws:\0:)2]}`, "foo bar\n"},
		{`print -r -- ${u[(ws.xy.)2]} ${#u[(w)1]} ${u[(ws.xy.i)b*]} ${u[(ws.xy.)1,(ws.xy.)2]} ${u[(ws.xy.r)c]}`, "b 7 4 axyb c\n"},
		{`s=:a::b:; print -r -- "<${s[(ws.:.)1]}><${s[(ws.:.)2]}><${s[(ws.:.)3]}><${s[(ws.:.)9]}>"`, "<a><b><><>\n"},
		{`s="aa bb cc"; s[(w)2]=ZZ; print -r -- $s`, "aa ZZ cc\n"},
		// An array counts elements whatever the group says.
		{`a=(x y); print -r -- ${a[(w)2]} ${a[(pi)y]}`, "y 2\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

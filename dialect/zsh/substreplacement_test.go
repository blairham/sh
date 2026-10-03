// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestTheSubstitutionModifiersReplacementExpandsWhereItLands pins that the
// replacement of `:s` in an unquoted expansion is spliced into the value and
// then has its own `$`s expanded, so a bare name reads on into the value
// after it; quotes in it are removed and protect what they hold; and in a
// double-quoted expansion nothing is expanded (#5151, a chunk of
// D04parameter.ztst). Measured 2026-10-03 on zsh 5.9.2 under `-f`.
func TestTheSubstitutionModifiersReplacementExpandsWhereItLands(t *testing.T) {
	const setup = "b=Q\n"
	for _, tc := range []struct{ src, want string }{
		{`a=(xax yay); print ${a:gs/a/${b}/}`, "xQx yQy\n"},
		{`s=xa.y; print ${s:s/a/$b/} $s:s/a/$b/`, "xQ.y xQ.y\n"},
		{`s=xay; by=W; print ${s:s/a/$b/}`, "xW\n"},
		{`s=xaay; print ${s:gs/a/$b/}`, "xQ\n"},
		{`set -- a b c d e f g h i j k l; s=xa2y; print ${s:s/a/$1/}`, "xly\n"},
		{`s=xa.y; print ${s:s/a/$b&/} ${s:s/a/&$b/}`, "x.y xaQ.y\n"},
		{`s=xa.y; b='&'; print ${s:s/a/$b/}`, "x&.y\n"},
		{`s=xay; print ${s:s/a/$(echo C)/} ${s:s/a/${b:l}/}`, "xCy xqy\n"},
		{`s=xay; print ${s:s/a/'q'/} ${s:s/a/x"$b"x/}`, "xqy xx\n"},
		{`s=xay; print ${s:s/a/\$b/} ${s:s/a/'$b'/}`, "x$by x$by\n"},
		{`s='xa$c'; c=Z; print ${s:s/a/${b}/}`, "xQ$c\n"},
		{`s=xa.y; print ${s:s/a/$b/}; b=R; print ${s:&}`, "xQ.y\nxR.y\n"},
		{`s=xa.y; print ${s:s/a/$/}`, "x$.y\n"},
		// Not the whole word: nothing is expanded, and the quotes still go.
		{`s=xa.y; print ${s:s/a/$b/}. .${s:s/a/$b/} ${s:s/a/'q'/}. ${s:s/a/$b/}""`, "x$b.y. .x$b.y xq.y. x$b.y\n"},
		// Double-quoted: nothing is expanded, and only the double quotes go.
		{`s=xay; print "${s:s/a/$b/}" "${s:s/a/'q'/}" "${s:s/a/"q"/}"`, "x$by x'q'y xqy\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

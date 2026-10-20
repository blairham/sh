// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestTheIFlagPicksWhichMatchASearchingTrimTakes pins `(I:n:)` beside `(S)`:
// one match per starting position, counted in the search's direction (#5151,
// a chunk of D04parameter.ztst). Measured 2026-10-02 on zsh 5.9.2
// (`/opt/homebrew/bin/zsh -f`, `LC_ALL=C`).
func TestTheIFlagPicksWhichMatchASearchingTrimTakes(t *testing.T) {
	const setup = "s=aXbXc; n=1\n"
	for _, tc := range []struct{ src, want string }{
		{`print -r -- ${(SI:1:)s#X*} ${(SI:2:)s#X*} ${(SI:3:)s#X*}`, "abXc aXbc aXbXc\n"},
		{`print -r -- ${(SI:1:)s##X*} ${(SI:2:)s##X*}`, "a aXb\n"},
		{`print -r -- ${(SI:1:)s%X*} ${(SI:2:)s%X*} ${(SI:3:)s%X*}`, "aXbc abXc aXbc\n"},
		{`print -r -- ${(SI:2:)s%%X*} ${(SI:3:)s%%X*}`, "a aXb\n"},
		{`print -r -- ${(BSI:2:)s%X*} ${(MSI:2:)s#X?}`, "2 Xc\n"},
		{`print -r -- ${(SI:n+1:)s#X*} ${(SI:0:)s#X*}`, "aXbc abXc\n"},
		// Without `(S)` there is one place a trim can match, and the flag
		// says nothing.
		{`print -r -- ${(I:2:)s#a?}`, "bXc\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

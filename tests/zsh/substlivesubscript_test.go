// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestASubscriptOnANestingKeepsTheLiveCharacters pins that a subscript naming
// positions on a nested expansion counts a live pattern character as the one
// character it is and leaves it live, while a search answers what it answered
// before (#5653). Measured 2026-10-03 on zsh 5.9.2 under -f, beside `xay` and
// `xby`.
func TestASubscriptOnANestingKeepsTheLiveCharacters(t *testing.T) {
	const setup = "print -n >xay; print -n >xby; s=xQy\n"
	for _, tc := range []struct{ src, want string }{
		{`print ${${s:s/Q/?/}[1,3]}`, "xay xby\n"},
		{`print ${${s:s/Q/?/}[2]}`, "zsh:2: no matches found: ?\n"},
		{`print ${${s:s/Q/?/}[2,-1]}`, "zsh:2: no matches found: ?y\n"},
		{`print ${${s:s/Q/?/}[-10,2]}`, "zsh:2: no matches found: x?\n"},
		{`i=1; print ${${s:s/Q/?/}[i+1]}`, "zsh:2: no matches found: ?\n"},
		{`t='x?y'; print ${${~t}[1,3]}`, "xay xby\n"},
		{`print ${(U)${s:s/Q/?/}[1,3]}`, "zsh:2: no matches found: X?Y\n"},
		{`a=(xQy xQb); print ${${a:s/Q/?/}[2]}`, "zsh:2: no matches found: x?b\n"},
		// A character the marks leave alone, and the searches, as before.
		{`print ${${s:s/Q/?/}[1]} ${${s:s/Q/?/}[5]}x ${${s:s/Q/?/}[(i)y]} ${${s:s/Q/?/}[(r)?]}`, "x x 3 x\n"},
		{`print "${${s:s/Q/?/}[2]}"`, "?\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

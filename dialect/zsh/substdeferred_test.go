// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestASubstitutionsReplacementExpandsAfterTheRestOfTheExpansion pins that the
// expansions a `:s` replacement wrote run on the *source* once every later
// modifier and case flag has, so they see what those did to the names
// (#5612). Measured 2026-10-03 on zsh 5.9.2 under `-f`.
func TestASubstitutionsReplacementExpandsAfterTheRestOfTheExpansion(t *testing.T) {
	const setup = "s=xa.y; b=Q\n"
	for _, tc := range []struct{ src, want string }{
		{`print ${(U)s:s/a/$b/} ${(C)s:s/a/$b/} ${s:s/a/$b/:u} ${(U)s:s/a/${b}/}`, "X.Y X.Y X.Y X.Y\n"},
		{`B=W; print ${(U)s:s/a/$b/} ${(L)s:s/a/$B/}`, "XW.Y xQ.y\n"},
		{`print ${(L)s:s/a/$b/}`, "xQ.y\n"},
		{`c=W; print ${s:s/a/$b/:s/b/c/} ${s:s/a/$b/:s/\$/D/}`, "xW.y xQ.y\n"},
		{`print ${#s:s/a/$b/} ${#s:s/a/${b}/}`, "5 7\n"},
		{`s=xa/y; print ${s:s/a/$b/:h}`, "xQ\n"},
		// The controls: no later modifier or flag, and a quoted expansion.
		{`print ${s:s/a/$b/} ${(o)s:s/a/$b/} "${s:s/a/$b/}"`, "xQ.y xQ.y x$b.y\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

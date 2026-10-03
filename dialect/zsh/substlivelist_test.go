// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestASubstitutionsLiveCharactersReachABareArrayAndTheParameters pins that
// the pattern characters a `:s` replacement writes stay live over a bare
// array name and over `$@` and `$*`, as over `${a[@]}` (#5639). Measured
// 2026-10-03 on zsh 5.9.2 under `-f`, beside `xay` and `xby`.
func TestASubstitutionsLiveCharactersReachABareArrayAndTheParameters(t *testing.T) {
	const setup = "print -n >xay; print -n >xby\n"
	for _, tc := range []struct{ src, want string }{
		{`a=(xQy); print ${a:gs/Q/?/}; set -- xQy; print ${@:s/Q/?/}`, "xay xby\nxay xby\n"},
		{`a=(xQy xQz); print ${a:s/Q/?/}`, "zsh:2: no matches found: x?z\n"},
		{`set -- xQy xQb; print ${*:s/Q/?/}`, "zsh:2: no matches found: x?b\n"},
		// Quoted, or assigned, the result is text.
		{`a=(xQy); print "${a:s/Q/?/}"; v=${a:s/Q/?/}; print $v`, "x?y\nx?y\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

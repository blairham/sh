// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestARangeKeepsItsBackslashes pins zsh's reading of a backslash in a
// substring range: kept for the arithmetic, except one before `$` (#5637).
// Measured 2026-10-03 on zsh 5.9.2 under `-f`.
func TestARangeKeepsItsBackslashes(t *testing.T) {
	const setup = "foo=abcdef\n"
	for _, tc := range []struct{ src, want string }{
		{
			`(: ${foo:0:\1}); (: "${foo:1:\1}"); (: ${foo:0:\"}); (: ${foo:0:$\"}); echo done`,
			"zsh:2: bad math expression: illegal character: \\\nzsh:2: bad math expression: illegal character: \\\nzsh:2: bad math expression: illegal character: \\\nzsh:2: bad math expression: illegal character: \\\ndone\n",
		},
		{`print -r -- ${foo:0:\$} ${foo:1:2}`, "abcdef bc\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

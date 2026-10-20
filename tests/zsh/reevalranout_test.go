// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestARereadValueThatRanOutInTheNameIsABadSubstitution: a value the `(e)`
// flag reads again whose `${` never closed is a `bad substitution` where it
// ran out before an operator, and `closing brace expected` once one was read
// — the command line's reader says the second for both. Measured 2026-10-02
// on zsh 5.9.2, the A01grammar chunk `Status on bad substitution in if
// without else` (#5138).
func TestARereadValueThatRanOutInTheNameIsABadSubstitution(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`a='${'; if : ${(e)a}; then echo x; fi`, "zsh:1: bad substitution\n"},
		{`a='${x'; : ${(e)a}`, "zsh:1: bad substitution\n"},
		{`a='${#x'; : ${(e)a}`, "zsh:1: bad substitution\n"},
		{`a='${(e)x'; : ${(e)a}`, "zsh:1: bad substitution\n"},
		{`a='${x:-'; : ${(e)a}`, "zsh:1: closing brace expected\n"},
		{`eval ': ${x'`, "(eval):1: closing brace expected\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), tc.src)
		if out != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}

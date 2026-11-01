// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A scalar `+=` written as a prefix over a table replaces it for the command,
// as the statement does here, rather than joining the table's text (#6902).
//
// Measured 2026-10-16 on zsh 5.9.2 `-f` from a file, byte for byte.
func TestAPrefixAppendOverATableReplacesIt(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ src, want string }{
		{`typeset -A h; h=(k v); h+=z eval "typeset -p h"; typeset -p h`, "typeset h=z\ntypeset -A h=( [k]=v )\n"},
		{"typeset -A h; h=(k v); h+=z /usr/bin/printenv h; f(){ typeset -p h; }; h+=z f", "z\nexport h=z\n"},
	} {
		if out, _ := answersRun(t, c.src); out != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
		}
	}
}

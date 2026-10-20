// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// TestAnArithmeticResultIsSplit is the standard's reading, and the answer of
// every column but zsh: an unquoted arithmetic result goes through field
// splitting like any other expansion. See
// interp.Semantics.ArithExpansionIsSplit.
//
// Measured 2026-10-02 on bash 5.3.20 and bash 3.2.57.
func TestAnArithmeticResultIsSplit(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`IFS=2; printf '<%s>\n' $((11*11))`, "<1>\n<1>\n"},
		{`IFS=2; printf '<%s>\n' "$((11*11))"`, "<121>\n"},
	} {
		out, _ := runBash(t, t.TempDir(), c.src+"\n")
		if out != c.want {
			t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
		}
	}
}

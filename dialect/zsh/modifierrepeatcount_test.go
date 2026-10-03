// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestTheRepeatCountIsAnExpressionInPairedDelimiters pins `:F<d>n<d>`: the
// count is an expression, its size is what counts, and its delimiters pair
// the way a flag's argument's do (#5151, a chunk of D04parameter.ztst).
// Measured 2026-10-02 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`, `LC_ALL=C`).
func TestTheRepeatCountIsAnExpressionInPairedDelimiters(t *testing.T) {
	const setup = "f=/one/two/three/four; n=2\n"
	for _, tc := range []struct{ src, want string }{
		{`print -r -- ${f:fh} ${f:F.1.h} ${f:F+2+h}`, "/ /one/two/three /one/two\n"},
		{`print -r -- ${f:F(3)h} ${f:F<4>h} ${f:F{5}h} ${f:F[2]h}`, "/one / / /one/two\n"},
		{`print -r -- ${f:F:1+1:h} ${f:F:n:h} ${f:F:-1:h}`, "/one/two /one/two /one/two/three\n"},
		{`print -r -- ${f:F:0:h} ${f:F:x:h} ${f:F:1:h:t}`, "/one/two/three/four /one/two/three/four three\n"},
		{`print -r -- ${f:F:1:}`, "zsh:2: unrecognized modifier `F'\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

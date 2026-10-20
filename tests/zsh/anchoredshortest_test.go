// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAFullyAnchoredShortestMatchIsTheWholeValue pins that under `(S)` a
// replacement anchored at both ends replaces the whole value, the only match
// it can have (#5151, a chunk of D04parameter.ztst). Measured 2026-10-03 on
// zsh 5.9.2 under `-f`.
func TestAFullyAnchoredShortestMatchIsTheWholeValue(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`v=abc; print ${(S)v/#%*/X} ${(S)v//#%(*)/X} ${(SU)v/#%*/x}`, "X X X\n"},
		{`setopt extendedglob; v=abc; print ${(S)v//#%((#b)(*))/X} $match[1]`, "X abc\n"},
		{`v=aba; a=(ab cd); print ${(S)v//#%a*/X} ${(S)a/#%*/Y}`, "X Y Y\n"},
		// The controls: no whole match, and one end anchored.
		{`v=abc; print ${(S)v//#%b/X} ${(S)v/#a*/X} ${(S)v//%*/X}`, "abc Xbc abcX\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

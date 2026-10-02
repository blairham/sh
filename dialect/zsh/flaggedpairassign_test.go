// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAFlaggedPairOnTheLeftNamesASpan pins that a pair whose ends carry flag
// groups replaces the span the two ends name on the left of an assignment, as
// the same pair reads it on the right (#5152). Measured 2026-10-02 on zsh
// 5.9.2 (`/opt/homebrew/bin/zsh -f`, `LC_ALL=C`).
func TestAFlaggedPairOnTheLeftNamesASpan(t *testing.T) {
	const setup = "a=(p q r s); s=hello\n"
	for _, tc := range []struct{ src, want string }{
		{`a[(r)q,(r)r]=(x y); print -r -- $a`, "p x y s\n"},
		{`a[(R)nf,(r)nf]=(x y); print -r -- $a`, "x y\n"},
		{`a[(R)nf,(r)nf]=Z; print -r -- $a`, "Z\n"},
		{`a[(r)q,3]=Z; print -r -- $a`, "p Z s\n"},
		{`a[1,(r)q]=Z; print -r -- $a`, "Z r s\n"},
		{`a[2,(i)r]=Z; print -r -- $a`, "p Z s\n"},
		{`a[(r)r,(r)q]=Z; print -r -- $a`, "p q Z r s\n"},
		{`a[(r)q,(r)nf]=Z; print -r -- $a`, "p Z\n"},
		{`a[(r)nf,(r)nf]=Z; print -r -- $a`, "p q r s Z\n"},
		{`a[(r)q,(r)r]=(); print -r -- $a`, "p s\n"},
		{`x=q; a[(r)$x,3]=Z; print -r -- $a`, "p Z s\n"},
		{`s[(r)l,(r)o]=Z; print -r -- $s`, "heZ\n"},
		{`s[(r)e,3]=Z; print -r -- $s`, "hZlo\n"},
		// `+=` reads no range: it appends to the element the second end names.
		{`a[(r)q,(r)r]+=Z; print -r -- $a`, "p q rZ s\n"},
		{`a[(r)q,(r)r]+=(X); print -r -- $a`, "p q r X s\n"},
		// An index letter cannot open a pair, and the line ends there.
		{`a[(i)q,3]=Z; print next`, "zsh:2: invalid subscript\n"},
		// The controls: a single flagged subscript, and an unflagged pair.
		{`a[(r)q]=Z; a[3,4]=W; print -r -- $a`, "p Z W\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

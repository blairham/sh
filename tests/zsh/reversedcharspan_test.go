// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAReversedSpanOverAStringKeepsWhatFollowsItsEnd pins that the string
// after the value resumes at the character after the span's end, wherever the
// end is, so a span that ends before it starts writes the characters between
// twice (#5477). Measured 2026-10-02 on zsh 5.9.2
// (`/opt/homebrew/bin/zsh -f`, `LC_ALL=C`).
func TestAReversedSpanOverAStringKeepsWhatFollowsItsEnd(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`s=abcd; s[3,1]=XY; print -r -- $s`, "abXYbcd\n"},
		{`s=abcd; s[4,2]=XY; print -r -- $s`, "abcXYcd\n"},
		{`s=abcd; s[3,0]=XY; print -r -- $s`, "abXYabcd\n"},
		{`s=abcd; s[-1,1]=XY; print -r -- $s`, "abcXYbcd\n"},
		{`s=abcd; s[9,1]=XY; print -r -- $s`, "abcdXYbcd\n"},
		{`s=abcd; typeset "s[3,1]"=XY; print -r -- $s`, "abXYbcd\n"},
		// The controls: an end one before the start, a forward span, and
		// `+=`, which reads no range.
		{`s=abc; s[3,2]=X; print -r -- $s`, "abXc\n"},
		{`s=abc; s[2,3]=X; print -r -- $s`, "aX\n"},
		{`s=abcd; s[3,1]+=XY; print -r -- $s`, "aXYbcd\n"},
		{`a=(p q r s); a[3,1]=XY; print -r -- $a`, "p q XY r s\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

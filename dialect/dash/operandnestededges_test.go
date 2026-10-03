// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import "testing"

// TestAnExpansionsEdgesInASubstitutedWordSeparateIt pins that the boundary an
// expansion inside the word a `-` or `+` substituted leaves at that word's
// end separates it from the text around the outer expansion (#5592).
// Measured 2026-10-03 against the reference for this dialect.
func TestAnExpansionsEdgesInASubstitutedWordSeparateIt(t *testing.T) {
	const f = "v=' p '; f(){ printf '<%s>' \"$@\"; echo; }\n"
	for _, tc := range []struct{ src, want string }{
		{`f x${u:-$v}y x${u:- $v}y`, "<x><p><y><x><p><y>\n"},
		{`f x${u:-a$v}y; w=1; f x${w:+$v}y`, "<xa><p><y>\n<x><p><y>\n"},
	} {
		got, _ := runDash(t, t.TempDir(), f+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

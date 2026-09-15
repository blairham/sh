// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A field past the C `int` is stored in the int here and what is left is used,
// on both routes and in silence.
//
// Measured 2026-09-15 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin` with a scratch
// HOME. The pair of widths is the point: both come to ten characters, and only
// the justification parts them, so a byte count could not tell padding from
// wrapping.
//
//	$ zsh -c 'printf "[%21474836470s]" x'    [x         ]   -10 as an int32
//	$ zsh -c 'printf "[%4294967306s]" x'     [         x]   +10
//
// The saturating read is the second half: `%99999999999999999999s` is past a
// 64-bit integer, saturates at LONG_MAX, truncates to -1, and leaves a width
// of one.
func TestAFieldBeyondAnIntWrapsHere(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`printf '[%21474836470s]' x`, "[x         ]"},
		{`printf '[%4294967306s]' x`, "[         x]"},
		{`printf '[%99999999999999999999s]' x`, "[x]"},
		// INT_MIN has no magnitude — negating it wraps back to itself — so
		// it is a width of nothing rather than one of two billion.
		{`printf '[%2147483648s]' x`, "[x]"},
		// A precision that wraps negative is C's "as if it were omitted".
		{`printf '[%.21474836470f]' 1`, "[1.000000]"},
		{`printf '[%.4294967306s]' xyz`, "[xyz]"},
		// The same on the star route, which this column does not part from
		// the written one — see dialect/dash, which does.
		{`printf '[%*s]' 21474836470 x`, "[x         ]"},
		{`printf '[%.*f]' 21474836470 1`, "[1.000000]"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s: said %q, want %q", tc.src, got, tc.want)
		}
	}
}

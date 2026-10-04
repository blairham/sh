// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// An exponent letter with no digit behind it belongs to the numeral and says
// nothing. Measured 2026-10-03 on ksh93u+ 2012-08-01, every row: the sign is
// taken with the letter, so `2e-x` is a refusal rather than a subtraction,
// and a byte after the letter is still a byte after the numeral.
func TestAnEmptyExponentIsPartOfTheNumeral(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`echo $(( 1.e ))`, "1\n"},
		{`echo $(( 1e ))`, "1\n"},
		{`echo $(( 1.e+ ))`, "1\n"},
		{`echo $(( 3e+ ))`, "3\n"},
		{`echo $(( 2.5e ))`, "2.5\n"},
		{`echo $(( 1.e*2 ))`, "2\n"},
		{`echo $(( 1.5e-1 ))`, "0.15\n"},
	} {
		if out, st := runKsh(t, t.TempDir(), tc.src); out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
	for _, src := range []string{`echo $(( 1.ex ))`, `x=1; echo $(( 2e-x ))`} {
		if _, st := runKsh(t, t.TempDir(), src); st == 0 {
			t.Errorf("%s succeeded, want a syntax error", src)
		}
	}
}

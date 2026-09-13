// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A `base#digits` numeral whose base is outside the range is refused **by the
// base**, whichever end of the range it is past and whatever the digits are.
//
// This shell stops a numeral at the first character its own base cannot use,
// and a base of one has no such character: the run after the `#` is empty
// however the numeral is spelled, so the digits were left standing where an
// operator belongs and the complaint named them. `$(( 1#5 ))` came out as an
// operator complaint about the `5` where zsh says `invalid base`, which sends
// a reader to look at their digits rather than at their base (#2575).
//
// Measured on zsh 5.9.2, 2026-09-13, `env -i PATH=/usr/bin:/bin` with `-c`.
// The upper end was already right, which is what makes the pair worth
// keeping: `37#5` and `1#5` are one sentence with a different number in it,
// and only the second was wrong.
func TestABaseOutsideTheRangeIsRefusedByName(t *testing.T) {
	const wording = "zsh:1: invalid base (must be 2 to 36 inclusive): "
	for _, tc := range []struct{ src, want string }{
		// The lower end, and the digits it has no room for.
		{`echo $(( 1#5 ))`, wording + "1\n"},
		{`echo $(( 1#z ))`, wording + "1\n"},
		{`echo $(( 1#Z ))`, wording + "1\n"},
		{`echo $(( 1#9z ))`, wording + "1\n"},
		{`echo $(( 1#0x10 ))`, wording + "1\n"},
		// A byte this shell refuses as part of any token on its own —
		// `$(( @ ))` is `illegal character: @` — is read into the numeral
		// once a base of one is in front of it, because the base is what
		// the complaint is about.
		{`echo $(( 1#@ ))`, wording + "1\n"},
		{`echo $(( 1#_ ))`, wording + "1\n"},
		// Nothing after the `#` at all, an operator after it, and the base
		// written padded or with a separator in it: the base is read from
		// the cleaned text, so all four name a plain `1`.
		{`echo $(( 1# ))`, wording + "1\n"},
		{`echo $(( 1#+2 ))`, wording + "1\n"},
		{`echo $(( 01#5 ))`, wording + "1\n"},
		{`echo $(( 1_#5 ))`, wording + "1\n"},
		// A base of one inside a larger expression still refuses at the
		// base rather than at the operator that follows the digits.
		{`echo $(( 1#5*2 ))`, wording + "1\n"},
		// The end of the range that was already right, in the same run so
		// that a change breaking it is caught here rather than elsewhere.
		{`echo $(( 37#5 ))`, wording + "37\n"},
		{`echo $(( 64#z ))`, wording + "64\n"},
	} {
		out, st := answersRun(t, tc.src)
		if out != tc.want || st != 1 {
			t.Errorf("%s gave %q at %d, want %q at 1", tc.src, out, st, tc.want)
		}
	}
}

// The control, and it is what says the change is about a base with no digits
// rather than about `base#` at large: a base that *has* an alphabet still
// ends its numeral at a digit outside it and still blames that digit.
func TestABaseInTheRangeStillEndsAtADigitItLacks(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`echo $(( 2#5 ))`, "zsh:1: bad math expression: operator expected at `5 '\n"},
		{`echo $(( 3#9 ))`, "zsh:1: bad math expression: operator expected at `9 '\n"},
	} {
		out, st := answersRun(t, tc.src)
		if out != tc.want || st != 1 {
			t.Errorf("%s gave %q at %d, want %q at 1", tc.src, out, st, tc.want)
		}
	}
	// And the numerals a legal base does read, so the change moved nothing
	// that was working.
	for _, tc := range []struct{ src, want string }{
		{`echo $(( 36#z ))`, "35\n"},
		{`echo $(( 0#5 ))`, "5\n"},
		{`echo $(( 16#ff ))`, "255\n"},
	} {
		out, st := answersRun(t, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

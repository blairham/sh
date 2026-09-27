// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// A plus form naming a width letter where the fill **rides** on a
// justification — see Runner.widthLettersTakenOffRiding and #4841.
//
// #4828 is the same question in the column where the letters are two
// attributes and each comes off on its own. Here they are three attributes
// behind three letters that do not name them one for one: `-Z` writes the
// fill *and* a right justification, so `+Z` takes both back and a bare `R`
// goes with a fill it was never written beside.
//
// Tests name axes and never shells.

func widthRidingRun(t *testing.T, src string, z DeclareZeroFillLetterPolicy) (string, string, int) {
	t.Helper()
	return declRun(t, src, func(s *Semantics) {
		s.DeclareOptions = "aAFgiLprRuxZ"
		s.DeclareOptionsTakingANumber = "FiLRZ"
		s.TypesetLocalNeedsKeywordFunction = No
		s.DeclareZeroFillLetter = z
		s.WidthJustificationPrecedence = WidthJustificationLastWrittenWins
		s.WidthNumberPrecedence = WidthNumberFirstWrittenWins
		s.AttributeRereadsTheValueItFinds = Yes
		s.DeclareNumberDetachedOnlyAtTheWordEnd = No
		// The store is the presentation in this column, which is what makes
		// the unwinding half of the question live at all: a letter coming
		// off has to take its own pad back or leave it, and the rows say
		// which.
		s.CaseAttributeFoldsWhenRead = No
	}, Diagnostics{})
}

// Both readings of the fill over one set of rows, because a suite that ran
// only the new one could not tell a fix from a hardcoding.
//
// Read through the presented value: `7    ` says a left justification is
// acting, `00007` a fill on a right one, and a bare `7` that the name carries
// nothing.
func TestAPlusFormOverARidingFillIsSettledBothWays(t *testing.T) {
	for _, tc := range []struct {
		zero                         DeclareZeroFillLetterPolicy
		lPlusZ, zPlusL, pairPlusZ    string
		pairPlusL, pairPlusR, zPlusR string
	}{
		{
			DeclareZeroFillLetterRidesOnTheJustification,
			"[7    ]\n", "[00007]\n", "[7    ]\n", "[7    ]\n", "[7    ]\n", "[7]\n",
		},
		// The other reading has no fill riding on anything, so `-Z` is a
		// justification of its own and the letter-wise answer #4828 records
		// applies instead. **One row parts them here and it is the last**:
		// `+R` takes a fill's own right justification with it under the
		// reading above and names a letter the name does not carry under
		// this one. The rest read alike because this harness stores the
		// *presentation* — the column that combines keeps the raw text and
		// re-presents it, so the value a letter reveals is its own question
		// and not this one. Where the two really part on the other rows is
		// the listing, which dialect/ksh measures against the binary.
		{
			DeclareZeroFillLetterCombinesWithTheLeftJustification,
			"[7    ]\n", "[00007]\n", "[7    ]\n", "[7    ]\n", "[7    ]\n", "[00007]\n",
		},
	} {
		t.Run(tc.zero.String(), func(t *testing.T) {
			for _, row := range []struct{ src, want string }{
				{`typeset -L5 v=7; typeset +Z v; printf "[%s]\n" "$v"`, tc.lPlusZ},
				{`typeset -Z5 v=7; typeset +L v; printf "[%s]\n" "$v"`, tc.zPlusL},
				{`typeset -Z5 -L5 v=7; typeset +Z v; printf "[%s]\n" "$v"`, tc.pairPlusZ},
				{`typeset -Z5 -L5 v=7; typeset +L v; printf "[%s]\n" "$v"`, tc.pairPlusL},
				{`typeset -L5 -Z5 v=7; typeset +R v; printf "[%s]\n" "$v"`, tc.pairPlusR},
				{`typeset -Z5 v=7; typeset +R v; printf "[%s]\n" "$v"`, tc.zPlusR},
			} {
				out, errs, st := widthRidingRun(t, row.src, tc.zero)
				if out != row.want || errs != "" || st != 0 {
					t.Errorf("%s = %q (stderr %q, status %d), want %q",
						row.src, out, errs, st, row.want)
				}
			}
		})
	}
}

// A fill that has lost its justification is a state of its own, and it
// presents nothing: the width still cuts a value to size and the leading
// blanks still come off, but nothing is laid down on either side.
func TestAFillWithNoJustificationPadsNothing(t *testing.T) {
	for _, row := range []struct{ name, src, want string }{
		{"a later value is not padded", `typeset -Z5 -L5 v=7; typeset +L v; v=9; printf "[%s]\n" "$v"`, "[9]\n"},
		{
			"but is still cut to the width",
			`typeset -Z5 -L5 v=7; typeset +L v; v=abcdefg; printf "[%s]\n" "$v"`, "[abcde]\n",
		},
		{
			"from the right, which is the left justification's side",
			`typeset -Z3 -L3 v=7; typeset +L v; v='  abcd'; printf "[%s]\n" "$v"`, "[abc]\n",
		},
		{
			"and its leading zeros are left where they are",
			`typeset -Z5 -L5 v=7; typeset +L v; v=00012; printf "[%s]\n" "$v"`, "[00012]\n",
		},
		{
			"and taking the fill off too leaves the text as it stood",
			`typeset -Z5 -L5 v=7; typeset +L v; typeset +Z v; printf "[%s]\n" "$v"`, "[7    ]\n",
		},
	} {
		t.Run(row.name, func(t *testing.T) {
			out, errs, st := widthRidingRun(t, row.src,
				DeclareZeroFillLetterRidesOnTheJustification)
			if out != row.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", row.src, out, errs, st, row.want)
			}
		})
	}
}

// `+R` over a fill takes the width again from what the value is left holding,
// and only where that value **begins with a digit** — which is the same first
// character test widthPadded makes for whether the pad is zeros at all.
//
// The last two rows are what say it is the digit and not the unwinding: both
// of those values lose a pad and keep their width, because the pad they lost
// was blanks.
func TestAPlusRightOverAFillTakesTheWidthAgain(t *testing.T) {
	for _, row := range []struct{ src, want string }{
		{`typeset -Z5 v=1a; typeset +R v; v=xyz; printf "[%s]\n" "$v"`, "[xy]\n"},
		{`typeset -Z5 v=1; typeset +R v; v=xyz; printf "[%s]\n" "$v"`, "[x]\n"},
		{`typeset -Z3 v=1; typeset +R v; v=xyz; printf "[%s]\n" "$v"`, "[x]\n"},
		{`typeset -Z5 v=12345; typeset +R v; v=abcdefg; printf "[%s]\n" "$v"`, "[abcde]\n"},
		{`typeset -Z5 v=ab; typeset +R v; v=abcdefg; printf "[%s]\n" "$v"`, "[abcde]\n"},
		{`typeset -Z5 v=-7; typeset +R v; v=abcdefg; printf "[%s]\n" "$v"`, "[abcde]\n"},
		// And `+L` is the control for that half: it leaves the width where it
		// found it and does not unwind, so the value keeps the blanks its
		// left justification laid down.
		{`typeset -Z5 -L5 v=7; typeset +L v; printf "[%s]\n" "$v"`, "[7    ]\n"},
	} {
		out, errs, st := widthRidingRun(t, row.src,
			DeclareZeroFillLetterRidesOnTheJustification)
		if out != row.want || errs != "" || st != 0 {
			t.Errorf("%s = %q (stderr %q, status %d), want %q", row.src, out, errs, st, row.want)
		}
	}
}

// What a plus form still takes off whole, and each of these agreed before any
// of the above: a letter that names every attribute the name carries leaves
// nothing, and a blank pad is not taken back where a zero one is.
func TestWhatARidingPlusFormStillTakesOffWhole(t *testing.T) {
	for _, row := range []struct{ name, src, want string }{
		{"the fill and its default justification", `typeset -Z4 v=7; typeset +Z v; printf "[%s]\n" "$v"`, "[7]\n"},
		{"a right justification through the fill's letter", `typeset -R5 v=7; typeset +Z v; printf "[%s]\n" "$v"`, "[    7]\n"},
		{"a right justification by its own letter", `typeset -R5 v=ab; typeset +R v; printf "[%s]\n" "$v"`, "[   ab]\n"},
		{"a left justification", `typeset -L5 v=ab; typeset +L v; printf "[%s]\n" "$v"`, "[ab   ]\n"},
	} {
		t.Run(row.name, func(t *testing.T) {
			out, errs, st := widthRidingRun(t, row.src,
				DeclareZeroFillLetterRidesOnTheJustification)
			if out != row.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", row.src, out, errs, st, row.want)
			}
		})
	}
}

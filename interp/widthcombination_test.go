// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// The width letters arriving and leaving on **separate** declarations — see
// Runner.widthLettersAreSeparable and #4828.
//
// The pair that stands together on one declaration is #4798's; this is the
// same pair reaching a name one letter at a time, and then coming apart
// again. Where the fill rides on a justification the two letters are one
// attribute, so there is no second letter for either question to be about and
// a declaration replaces what it finds.
//
// Tests name axes and never shells.

func widthAcrossRun(t *testing.T, src string, z DeclareZeroFillLetterPolicy) (string, string, int) {
	t.Helper()
	return declRun(t, src, func(s *Semantics) {
		s.DeclareOptions = "aAFgiLprRuxZ"
		s.DeclareOptionsTakingANumber = "FiLRZ"
		s.TypesetLocalNeedsKeywordFunction = No
		s.DeclareZeroFillLetter = z
		s.WidthJustificationPrecedence = WidthJustificationConflictLeavesNoWidth
		s.WidthNumberPrecedence = WidthNumberLastWrittenWins
		// A second declaration over a name that already holds something
		// re-reads what it finds, which every row here needs and which is a
		// question of its own.
		s.AttributeRereadsTheValueItFinds = Yes
		// The store is the text the assignment carried and the width acts on
		// the **read**, which is the column these rows were measured in: a
		// letter taken off has to reveal the raw value rather than its own
		// predecessor's padding. See Semantics.CaseAttributeFoldsWhenRead.
		s.CaseAttributeFoldsWhenRead = Yes
		s.DeclareNumberDetachedOnlyAtTheWordEnd = No
	}, Diagnostics{})
}

// Both answers over one set of rows, because a suite that ran only the new
// one could not tell a fix from a hardcoding.
//
// Read through the presented value: `7    ` says a left justification is
// acting, `00007` a zero fill, and `7` that the name carries no width at all.
// Under the reading where the letters are one attribute the second
// declaration simply replaces the first, which is what the right-hand column
// of every row says.
func TestASecondWidthLetterJoinsOrReplaces(t *testing.T) {
	for _, tc := range []struct {
		zero                   DeclareZeroFillLetterPolicy
		lThenZ, zThenL, narrow string
	}{
		{DeclareZeroFillLetterCombinesWithTheLeftJustification, "[7    ]\n", "[7    ]\n", "[7  ]\n"},
		{DeclareZeroFillLetterRidesOnTheJustification, "[00007]\n", "[7    ]\n", "[007]\n"},
	} {
		t.Run(tc.zero.String(), func(t *testing.T) {
			for _, row := range []struct{ src, want string }{
				{`typeset -L5 v=7; typeset -Z5 v; printf "[%s]\n" "$v"`, tc.lThenZ},
				{`typeset -Z5 v=7; typeset -L5 v; printf "[%s]\n" "$v"`, tc.zThenL},
				// The arriving declaration's number reaches both letters
				// where they join, and is the whole attribute's where they
				// do not.
				{`typeset -L5 v=7; typeset -Z3 v; printf "[%s]\n" "$v"`, tc.narrow},
			} {
				out, errs, st := widthAcrossRun(t, row.src, tc.zero)
				if out != row.want || errs != "" || st != 0 {
					t.Errorf("%s = %q (stderr %q, status %d), want %q",
						row.src, out, errs, st, row.want)
				}
			}
		})
	}
}

// Where the letters join, a plus form takes off the one it **names** and
// leaves the rest standing — and the value comes back through whatever is
// left, which is the half a listing-only fix would get wrong.
func TestAPlusFormTakesOffTheLetterItNames(t *testing.T) {
	for _, row := range []struct{ name, src, want string }{
		{
			"the fill goes and the justification stays",
			`typeset -L5 -Z5 v=7; typeset +Z v; printf "[%s]\n" "$v"`, "[7    ]\n",
		},
		{
			"the justification goes and the fill stays",
			`typeset -L5 -Z5 v=7; typeset +L v; printf "[%s]\n" "$v"`, "[00007]\n",
		},
		{
			"a letter the name does not carry takes nothing",
			`typeset -L5 -Z5 v=7; typeset +R v; printf "[%s]\n" "$v"`, "[7    ]\n",
		},
		{
			"nor over one letter alone",
			`typeset -L5 v=7; typeset +Z v; printf "[%s]\n" "$v"`, "[7    ]\n",
		},
		{
			"nor the other way round",
			`typeset -Z5 v=7; typeset +L v; printf "[%s]\n" "$v"`, "[00007]\n",
		},
		{
			"nor over the letter that joins neither",
			`typeset -R5 v=7; typeset +Z v; printf "[%s]\n" "$v"`, "[    7]\n",
		},
		// And the letters the name does carry really do come off, which is
		// what keeps the rows above a removal rather than a no-op.
		{"the letter it carries", `typeset -L5 v=7; typeset +L v; printf "[%s]\n" "$v"`, "[7]\n"},
		{"the fill alone", `typeset -Z5 v=7; typeset +Z v; printf "[%s]\n" "$v"`, "[7]\n"},
		{"the right one", `typeset -R5 v=7; typeset +R v; printf "[%s]\n" "$v"`, "[7]\n"},
		// Both letters named on one declaration leave nothing.
		{"both at once", `typeset -L5 -Z5 v=7; typeset +L +Z v; printf "[%s]\n" "$v"`, "[7]\n"},
	} {
		t.Run(row.name, func(t *testing.T) {
			out, errs, st := widthAcrossRun(t, row.src,
				DeclareZeroFillLetterCombinesWithTheLeftJustification)
			if out != row.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", row.src, out, errs, st, row.want)
			}
		})
	}
}

// The controls, and each agreed before the answer above existed.
//
// `R` joins nothing in either direction, so a declaration writing it replaces
// what stood and one arriving over it is replaced in turn; and a second
// declaration writing the letter the name already carries replaces rather
// than accumulating, which is what keeps this about the *pair*.
func TestWhatTheJoiningAnswerDoesNotReach(t *testing.T) {
	for _, row := range []struct{ name, src, want string }{
		{"a right justification replaces a left", `typeset -L5 v=7; typeset -R3 v; printf "[%s]\n" "$v"`, "[  7]\n"},
		{"and replaces a fill", `typeset -Z5 v=7; typeset -R3 v; printf "[%s]\n" "$v"`, "[  7]\n"},
		{"and is replaced by a fill", `typeset -R5 v=7; typeset -Z3 v; printf "[%s]\n" "$v"`, "[007]\n"},
		{"and by a left justification", `typeset -R5 v=7; typeset -L3 v; printf "[%s]\n" "$v"`, "[7  ]\n"},
		{"the same letter twice", `typeset -L5 v=ab; typeset -L3 v; printf "[%s]\n" "$v"`, "[ab ]\n"},
		{
			"a conflicting pair contributes nothing",
			`typeset -L5 v; typeset -L5 -R5 v; v=7; printf "[%s]\n" "$v"`, "[7    ]\n",
		},
	} {
		t.Run(row.name, func(t *testing.T) {
			out, errs, st := widthAcrossRun(t, row.src,
				DeclareZeroFillLetterCombinesWithTheLeftJustification)
			if out != row.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", row.src, out, errs, st, row.want)
			}
		})
	}
}

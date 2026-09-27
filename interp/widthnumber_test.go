// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// Two width **numbers** on one declaration — see Semantics.WidthNumberPrecedence
// and #4827.
//
// A separate question from which letter survives, and the two point opposite
// ways in one column, which is the whole reason there are two axes: the
// letter is the last written there and the number is the first.
//
// Tests name axes and never shells.

func widthNumberRun(t *testing.T, src string, p WidthNumberPrecedencePolicy) (string, string, int) {
	t.Helper()
	return declRun(t, src, func(s *Semantics) {
		s.DeclareOptions = "aAFgiLprRuxZ"
		s.DeclareOptionsTakingANumber = "FiLRZ"
		s.TypesetLocalNeedsKeywordFunction = No
		// The fill rides on a justification, which is the column the first-
		// written answer was measured in, and it is what lets one
		// declaration carry two numbers on two letters that both survive.
		s.DeclareZeroFillLetter = DeclareZeroFillLetterRidesOnTheJustification
		s.WidthJustificationPrecedence = WidthJustificationLastWrittenWins
		s.WidthNumberPrecedence = p
		// A second declaration over a name that already holds something
		// re-reads what it finds, which the "starts over" control needs and
		// which is a question of its own.
		s.AttributeRereadsTheValueItFinds = Yes
		// A detached number reaches its letter wherever the letter stands,
		// so every row below really writes two numbers. The other answer
		// discards the rest of the word — the control at the foot of this
		// file.
		s.DeclareNumberDetachedOnlyAtTheWordEnd = No
	}, Diagnostics{})
}

// Both answers over one set of rows, because a suite that ran only the new
// one could not tell a fix from a hardcoding.
//
// Read through the *presented value* rather than through the listing, because
// the width is what the padding counts: `7  ` is three characters wide and
// `7    ` five, so the row says which number the name kept without the
// listing style having to be in the question.
func TestTwoWidthNumbersAreSettledBothWays(t *testing.T) {
	for _, tc := range []struct {
		policy                         WidthNumberPrecedencePolicy
		lz, zl, rz, zr, ll, rl, triple string
	}{
		{
			WidthNumberFirstWrittenWins,
			"[7    ]\n", "[7  ]\n", "[00007]\n", "[007]\n", "[7    ]\n", "[7  ]\n", "[7    ]\n",
		},
		{
			WidthNumberLastWrittenWins,
			"[7  ]\n", "[7    ]\n", "[007]\n", "[00007]\n", "[7  ]\n", "[7    ]\n", "[7]\n",
		},
	} {
		t.Run(tc.policy.String(), func(t *testing.T) {
			for _, row := range []struct{ src, want string }{
				{`typeset -L5 -Z3 v=7; printf "[%s]\n" "$v"`, tc.lz},
				{`typeset -Z3 -L5 v=7; printf "[%s]\n" "$v"`, tc.zl},
				{`typeset -R5 -Z3 v=7; printf "[%s]\n" "$v"`, tc.rz},
				{`typeset -Z3 -R5 v=7; printf "[%s]\n" "$v"`, tc.zr},
				{`typeset -L5 -L3 v=7; printf "[%s]\n" "$v"`, tc.ll},
				{`typeset -R3 -L5 v=7; printf "[%s]\n" "$v"`, tc.rl},
				{`typeset -L5 -Z3 -L1 v=7; printf "[%s]\n" "$v"`, tc.triple},
			} {
				out, errs, st := widthNumberRun(t, row.src, tc.policy)
				if out != row.want || errs != "" || st != 0 {
					t.Errorf("%s = %q (stderr %q, status %d), want %q",
						row.src, out, errs, st, row.want)
				}
			}
		})
	}
}

// The letter is still the **last** written where the number is the first, and
// that is the row the two rules have to be two rules to produce: `-R3 -L5` is
// the `L` from the second option word presented in the `3` from the first.
//
// Asserted on the listing as well as on the value, because the value alone
// cannot say which letter is carrying the width once both numbers are equal.
func TestTheLetterAndTheNumberComeFromOppositeEnds(t *testing.T) {
	for _, row := range []struct{ src, want string }{
		{`typeset -R3 -L5 v=7; typeset -p v`, `declare -- v="7  "` + "\n"},
		{`typeset -L3 -R5 v=7; typeset -p v`, `declare -- v="  7"` + "\n"},
		{`typeset -L5 -Z3 v=7; typeset -p v`, `declare -- v="7    "` + "\n"},
	} {
		out, errs, st := widthNumberRun(t, row.src, WidthNumberFirstWrittenWins)
		if out != row.want || errs != "" || st != 0 {
			t.Errorf("%s = %q (stderr %q, status %d), want %q", row.src, out, errs, st, row.want)
		}
	}
}

// A written zero names no width under either answer: it leaves the width to
// be learned from the first value, so it neither settles the question nor is
// settled by it.
//
// These rows are why the first-written answer cannot be spelled "a number
// already stored": `-L0 -L4` would then keep the zero and learn 2 from `ab`,
// which is not what was measured.
func TestAWrittenZeroNamesNoWidthUnderEitherAnswer(t *testing.T) {
	for _, tc := range []struct {
		policy             WidthNumberPrecedencePolicy
		zeroThen, thenZero string
	}{
		{WidthNumberFirstWrittenWins, "[ab  ]\n", "[ab  ]\n"},
		// The last written really is the zero here, so the width comes from
		// the value instead — which is the one row the two answers part on
		// for a reason other than the order.
		{WidthNumberLastWrittenWins, "[ab  ]\n", "[ab]\n"},
	} {
		t.Run(tc.policy.String(), func(t *testing.T) {
			for _, row := range []struct{ src, want string }{
				{`typeset -L0 -L4 v=ab; printf "[%s]\n" "$v"`, tc.zeroThen},
				{`typeset -L4 -L0 v=ab; printf "[%s]\n" "$v"`, tc.thenZero},
			} {
				out, errs, st := widthNumberRun(t, row.src, tc.policy)
				if out != row.want || errs != "" || st != 0 {
					t.Errorf("%s = %q (stderr %q, status %d), want %q",
						row.src, out, errs, st, row.want)
				}
			}
		})
	}
}

// What the first-written answer must not reach, and every row here agreed
// before it existed — which is what makes them controls rather than coverage.
//
// A letter written bare takes the standing number whichever answer is in
// force, so these rows are produced identically by both and are evidence
// about neither; and a second *declaration* starts the question over, because
// the rule is about one declaration's option words.
func TestWhatTheFirstNumberAnswerDoesNotReach(t *testing.T) {
	for _, row := range []struct{ name, src, want string }{
		{"a bare letter takes the standing number", `typeset -L -Z5 v=7; printf "[%s]\n" "$v"`, "[7    ]\n"},
		{"and in the other order", `typeset -Z5 -L v=7; printf "[%s]\n" "$v"`, "[7    ]\n"},
		{"a bare letter written last", `typeset -L3 -R v=7; printf "[%s]\n" "$v"`, "[  7]\n"},
		{"a second declaration starts over", `typeset -L5 v=ab; typeset -L3 v; printf "[%s]\n" "$v"`, "[ab ]\n"},
	} {
		t.Run(row.name, func(t *testing.T) {
			out, errs, st := widthNumberRun(t, row.src, WidthNumberFirstWrittenWins)
			if out != row.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", row.src, out, errs, st, row.want)
			}
		})
	}
}

// And the spelling where the second number never arrives: a **detached**
// number reaching a letter that does not end its option word takes the number
// and discards the rest of the word, so `typeset -LZ 5` writes one number and
// there is nothing for either answer to choose between.
func TestADiscardedLetterCarriesNoSecondNumber(t *testing.T) {
	for _, policy := range []WidthNumberPrecedencePolicy{
		WidthNumberFirstWrittenWins, WidthNumberLastWrittenWins,
	} {
		t.Run(policy.String(), func(t *testing.T) {
			src := `typeset -LZ 5 v=7; printf "[%s]\n" "$v"`
			out, errs, st := declRun(t, src, func(s *Semantics) {
				s.DeclareOptions = "aAFgiLprRuxZ"
				s.DeclareOptionsTakingANumber = "FiLRZ"
				s.TypesetLocalNeedsKeywordFunction = No
				s.DeclareZeroFillLetter = DeclareZeroFillLetterRidesOnTheJustification
				s.WidthJustificationPrecedence = WidthJustificationLastWrittenWins
				s.WidthNumberPrecedence = policy
				s.DeclareNumberDetachedOnlyAtTheWordEnd = Yes
			}, Diagnostics{})
			if out != "[7    ]\n" || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", src, out, errs, st, "[7    ]\n")
			}
		})
	}
}

// The answer is the **width** letters' and not every number an option word
// carries, which is a discriminator rather than coverage: a declaration
// writing two precisions keeps the last one in the same column that keeps the
// first width. Measured 2026-09-27 on ksh93u+ 2012-08-01 under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` — `typeset -F5 -F3 v=1.5` is `typeset -F 3
// v=1.500`, `typeset -E5 -E3 v=1.5` is `typeset -E 3`, and `typeset -i8 -i10
// c=9` is base ten — so a change that had moved readOptionNumber's other two
// branches with it would be wrong there and right here.
//
// Run through the float harness because a float attribute needs floating
// point in the arithmetic to be given a value at all.
func TestTheFirstNumberAnswerIsTheWidthLettersAlone(t *testing.T) {
	src := `typeset -F5 -F3 v=1.5; printf "[%s]\n" "$v"`
	out, errs, st := floatRun(t, src, func(s *Semantics) {
		withFloatLetter(s)
		s.DeclareOptions = "aAiFLprRxZ"
		s.DeclareOptionsTakingANumber = "FLRZ"
		s.DeclareZeroFillLetter = DeclareZeroFillLetterRidesOnTheJustification
		s.WidthJustificationPrecedence = WidthJustificationLastWrittenWins
		s.WidthNumberPrecedence = WidthNumberFirstWrittenWins
	}, Diagnostics{})
	if out != "[1.500]\n" || errs != "" || st != 0 {
		t.Errorf("%s = %q (stderr %q, status %d), want %q", src, out, errs, st, "[1.500]\n")
	}
}

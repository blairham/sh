// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// Where a name's output base is written down, for the one axis that takes a
// base from the value assigned rather than from an option letter. A radix at
// the front of an assignment's text has always been read; a radix standing
// *inside* an expression is the same radix and was read by nobody, which is
// #3670.
//
// Named for the axis and never for a shell, as everything in this package is.

// learnsBases is the vector these tests share: a shell that takes a name's
// output base from the value assigned to it, with the two questions an
// expression's own numerals raise answered so that each row reaches the one
// this file is about.
func learnsBases(learns Answer) func(*Semantics) {
	return func(s *Semantics) {
		s.IntegerBaseComesFromTheValueAssigned = learns
		s.IntegerBaseDigits = "0123456789abcdefghijklmnopqrstuvwxyz"
		// A leading zero inside an expression is octal here, which is the
		// reading the `010` rows are measured under.
		s.ArithLeadingZeroIsOctal = Yes
		s.IntegerAssignmentReadsALeadingZeroAsDecimal = No
		// The attribute arriving over a name that already holds text is a
		// question of its own, answered here so the re-read row reaches the
		// base rather than a refusal.
		s.AttributeRereadsTheValueItFinds = Yes
	}
}

// An operator in the text does not hide the radix that stands beside it. The
// six rows are the shapes a radix can be buried in: an operand of a binary
// operator either side, a branch of a conditional, one inside parentheses,
// a `base#digits` spelling rather than a prefix, and the bare leading zero
// the same expression can carry instead.
func TestAnIntegerNameLearnsTheRadixInsideAnExpression(t *testing.T) {
	const src = `typeset -i a=1+0x1f; echo "1[$a]"
typeset -i b=0x1f+1; echo "2[$b]"
typeset -i c=1?0x10:2; echo "3[$c]"
typeset -i d='(1+0x1f)'; echo "4[$d]"
typeset -i e=1+16#ff; echo "5[$e]"
typeset -i f=1+010; echo "6[$f]"`
	for _, tc := range []struct {
		learns Answer
		want   string
	}{
		{Yes, "1[16#20]\n2[16#20]\n3[16#10]\n4[16#20]\n5[16#100]\n6[8#11]\n"},
		{No, "1[32]\n2[32]\n3[16]\n4[32]\n5[256]\n6[9]\n"},
	} {
		out, errs, st := integerRun(t, src, learnsBases(tc.learns), Diagnostics{})
		if out != tc.want || st != 0 || errs != "" {
			t.Errorf("%v = %q (stderr %q, status %d), want %q",
				tc.learns, out, errs, st, tc.want)
		}
	}
}

// Every route that folds a text through the integer attribute learns the same
// way, which is the reason the reading lives in learnIntegerBase rather than
// beside the declaration: a two-step assignment, an append, and the re-read a
// name already holding a value takes when the attribute arrives.
func TestEveryIntegerRouteLearnsTheRadixInsideAnExpression(t *testing.T) {
	const src = `typeset -i a; a=1+0x1f; echo "1[$a]"
typeset -i b=1; b+=1+0x1f; echo "2[$b]"
c=1+0x10; typeset -i c; echo "3[$c]"`
	for _, tc := range []struct {
		learns Answer
		want   string
	}{
		{Yes, "1[16#20]\n2[16#21]\n3[16#11]\n"},
		{No, "1[32]\n2[33]\n3[17]\n"},
	} {
		out, errs, st := integerRun(t, src, learnsBases(tc.learns), Diagnostics{})
		if out != tc.want || st != 0 || errs != "" {
			t.Errorf("%v = %q (stderr %q, status %d), want %q",
				tc.learns, out, errs, st, tc.want)
		}
	}
}

// The base is the *name's* once learned, exactly as one learned from a
// literal is: a later plain number reads back in it, and a second expression
// carrying a different radix does not take it over.
func TestARadixLearnedInsideAnExpressionBelongsToTheName(t *testing.T) {
	const src = `typeset -i a; a=1+0x1f; a=5; echo "1[$a]"
typeset -i b; b=1+0x1f; b=1+010; echo "2[$b]"`
	out, errs, st := integerRun(t, src, learnsBases(Yes), Diagnostics{})
	const want = "1[16#5]\n2[16#9]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// An expression holding no radix teaches nothing, which is the case the fast
// path in front of the parse is there for. The value is right in every row
// either way — the question is only what the name renders in afterwards.
func TestAnExpressionWithNoRadixTeachesNoBase(t *testing.T) {
	const src = `typeset -i a=1+2; echo "1[$a]"
typeset -i b=5; echo "2[$b]"
typeset -i c=-7; echo "3[$c]"
typeset -i d; d=x+1; echo "4[$d]"`
	for _, learns := range []Answer{Yes, No} {
		out, errs, st := integerRun(t, src, learnsBases(learns), Diagnostics{})
		const want = "1[3]\n2[5]\n3[-7]\n4[1]\n"
		if out != want || st != 0 || errs != "" {
			t.Errorf("%v = %q (stderr %q, status %d), want %q",
				learns, out, errs, st, want)
		}
	}
}

// A text that will not parse is still the evaluation's to report: the walk in
// front of it says nothing and leaves the name alone.
func TestAnUnparsableExpressionIsStillTheEvaluationsToReport(t *testing.T) {
	out, errs, st := integerRun(t, `typeset -i a=0x1zz; echo "[$a]"`,
		learnsBases(Yes), Diagnostics{})
	if st == 0 || errs == "" || out != "" {
		t.Errorf("= %q (stderr %q, status %d), want a refusal and no output",
			out, errs, st)
	}
}

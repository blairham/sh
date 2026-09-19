// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// Semantics.TestThreeWordsNegateBeforeAConnective: a three-word `test` whose
// first word is `!` is read as a negation of the other two, ahead of the
// reading that takes the middle word as a connective over two strings.
//
// The rows are a measurement of two columns and are asked here by the axis
// rather than by any shell's name — the axis's own documentation says where
// they were taken and under what.
//
// Graded on the **status**, because that is the whole of the answer: every
// row either answers yes or no or refuses, and none of the readings prints
// anything on the way to a yes or a no.

func negateBeforeConnective(yes Answer) func(*Runner) {
	return func(r *Runner) {
		s := *r.Semantics
		s.TestThreeWordsNegateBeforeAConnective = yes
		r.Semantics = &s
	}
}

// threeWordRows are the shapes, with the status each reading gives.
//
// `first` is the column that negates ahead of the connective and `after` is
// the one that does not. A row marked a control is one the two readings
// answer alike, and the controls are what say the axis is confined to the one
// shape rather than being "a leading `!` stops working".
var threeWordRows = []struct {
	src          string
	first, after int
	control      bool
}{
	// The shape itself: three words, a leading `!`, a connective in the
	// middle. Negating first makes each of these the two-word reading of the
	// last two words with a `!` in front of it, and `-a` is not a unary
	// operator, so the two-word reading refuses.
	{src: `test ! -a x`, first: 2, after: 0},
	{src: `test ! -o x`, first: 2, after: 0},
	{src: `test ! -a ""`, first: 2, after: 1},
	{src: `test ! -o ""`, first: 2, after: 0},
	{src: `test ! -a -n`, first: 2, after: 0},

	// The binary reading comes first in **both**, which is the row that
	// keeps this from being a rule about a leading `!`: `=` and `-eq` take
	// the `!` as their left operand before either reading is reached.
	{src: `test ! = x`, first: 1, after: 1, control: true},
	{src: `test ! = !`, first: 0, after: 0, control: true},
	{src: `test ! != x`, first: 0, after: 0, control: true},

	// A leading `!` with no connective in the middle is a negation in both,
	// because the branch the axis orders is never reached.
	{src: `test ! -n x`, first: 1, after: 1, control: true},
	{src: `test ! -z x`, first: 0, after: 0, control: true},
	{src: `test ! -n ""`, first: 0, after: 0, control: true},

	// The connective with something other than `!` in front of it, which is
	// the guard scripts actually write.
	{src: `test x -a y`, first: 0, after: 0, control: true},
	{src: `test x -a ""`, first: 1, after: 1, control: true},
	{src: `test "" -o x`, first: 0, after: 0, control: true},
	{src: `test x -o ""`, first: 0, after: 0, control: true},

	// And the counts on either side. Two words and four words are read by
	// rules of their own, and a four-word `!` in front of a connective
	// negates the three words behind it in both columns.
	{src: `test ! x`, first: 1, after: 1, control: true},
	{src: `test ! ""`, first: 0, after: 0, control: true},
	{src: `test ! x -a y`, first: 1, after: 1, control: true},
	{src: `test ! "" -o y`, first: 1, after: 1, control: true},
}

func TestThreeWordsLedByANegationReadItBeforeTheConnective(t *testing.T) {
	for _, tc := range threeWordRows {
		t.Run(tc.src, func(t *testing.T) {
			out, st := run(t, tc.src, negateBeforeConnective(Yes))
			if st != tc.first {
				t.Errorf("%s = %d, want %d (output %q)", tc.src, st, tc.first, out)
			}
		})
	}
}

// The other answer over the same rows, which is the mutation: the five that
// are not controls move and the fifteen controls do not.
func TestThreeWordsReadTheConnectiveBeforeALeadingNegation(t *testing.T) {
	moved := 0
	for _, tc := range threeWordRows {
		out, st := run(t, tc.src, negateBeforeConnective(No))
		if st != tc.after {
			t.Errorf("%s = %d, want %d (output %q)", tc.src, st, tc.after, out)
		}
		if tc.first != tc.after {
			moved++
		}
		if tc.control && tc.first != tc.after {
			t.Errorf("%s is marked a control and moves with the axis", tc.src)
		}
	}
	// A floor rather than the figure, so that the table stops measuring the
	// axis the moment nothing moves with it.
	if moved < 5 {
		t.Errorf("%d of %d rows moved with the axis, want at least 5", moved, len(threeWordRows))
	}
}

// And the four-word rule reads these three words the way the axis says,
// rather than by their spelling.
//
// Semantics.TestFourWordsNegateANegationOnce folds a leading `!` into a
// negation behind it. Whether `! -a x` *is* a negation is what this axis
// decides, so the two have to be asked together: with the negation read first
// the four words are the refusal the three give, and with the connective read
// first they are that guard negated.
func TestTheFourWordFoldSeesWhatThisAxisMakesOfTheThreeWords(t *testing.T) {
	both := func(neg, fold Answer) func(*Runner) {
		return func(r *Runner) {
			s := *r.Semantics
			s.TestThreeWordsNegateBeforeAConnective = neg
			s.TestFourWordsNegateANegationOnce = fold
			r.Semantics = &s
		}
	}
	if _, st := run(t, `test ! ! -a x`, both(Yes, Yes)); st != 2 {
		t.Errorf("negation first: status %d, want 2 — the three words behind the `!` refuse", st)
	}
	if _, st := run(t, `test ! ! -a x`, both(No, No)); st != 1 {
		t.Errorf("connective first: status %d, want 1 — the guard, negated", st)
	}
}

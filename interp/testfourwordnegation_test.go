// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// Semantics.TestFourWordsNegateANegationOnce: a four-word `test` led by `!`
// takes the three-word reading of the rest and does not negate it again,
// where the rest is itself a negation.
//
// The rows are a measurement of one column and are asked here by the axis
// rather than by the shell's name — the axis's own documentation says where
// they were taken and under what.
//
// Every row is graded on the **status**, because that is the whole of the
// answer: both readings exit without output, so a case that looked at the
// text would be looking at nothing.

func negateOnce(yes Answer) func(*Runner) {
	return func(r *Runner) {
		s := *r.Semantics
		s.TestFourWordsNegateANegationOnce = yes
		r.Semantics = &s
	}
}

// fourWordRows are the four-word shapes, with the status each reading gives.
//
// `once` is the column that drops the outer negation and `twice` is the other
// four, and a row where the two are equal is a control: it is a four-word `!`
// that both readings negate, and it says the axis is not simply "a leading
// `!` stops working".
var fourWordRows = []struct {
	src         string
	once, twice int
	control     bool
}{
	// The negations. The `once` column is what `[ ! -n x ]` and its
	// neighbors answer on their own, three words at a time.
	{src: `test ! ! -n x`, once: 1, twice: 0},
	{src: `test ! ! -n ""`, once: 0, twice: 1},
	{src: `test ! ! -z x`, once: 0, twice: 1},
	{src: `test ! ! -z ""`, once: 1, twice: 0},
	{src: `test ! ! -n -n`, once: 1, twice: 0},
	{src: `test ! ! ! x`, once: 0, twice: 1},
	{src: `test ! ! ! ""`, once: 1, twice: 0},

	// The controls at the same count. A four-word `!` in front of something
	// that is **not** a negation negates in every column.
	{src: `test ! x = x`, once: 1, twice: 1, control: true},
	{src: `test ! x = y`, once: 0, twice: 0, control: true},
	{src: `test ! x -a y`, once: 1, twice: 1, control: true},
	{src: `test ! "" -o y`, once: 1, twice: 1, control: true},
	// The sharpest of them: two leading `!`s whose three-word remainder is a
	// *string comparison* rather than a negation, because `=` takes the
	// first `!` as its left operand. A rule reading "two leading `!`s
	// cancel at four words" gets this one wrong.
	{src: `test ! ! = x`, once: 0, twice: 0, control: true},

	// And the counts on either side, which both readings answer alike:
	// three words and five words negate as a recursive reading predicts.
	{src: `test ! ! x`, once: 0, twice: 0, control: true},
	{src: `test ! ! ""`, once: 1, twice: 1, control: true},
	{src: `test ! -n x`, once: 1, twice: 1, control: true},
	{src: `test ! ! x = x`, once: 0, twice: 0, control: true},
	{src: `test ! ! ! -n x`, once: 1, twice: 1, control: true},
	{src: `test ! ! ! ! -n x`, once: 0, twice: 0, control: true},
}

func TestFourWordsLedByANegationNegateTheNegationBehindThemOnce(t *testing.T) {
	for _, tc := range fourWordRows {
		t.Run(tc.src, func(t *testing.T) {
			out, st := run(t, tc.src, negateOnce(Yes))
			if st != tc.once {
				t.Errorf("%s = %d, want %d (stdout %q)", tc.src, st, tc.once, out)
			}
		})
	}
}

// The other answer over the same rows, which is the mutation: the seven that
// are not controls move and the twelve controls do not.
func TestFourWordsLedByANegationNegateTwiceWhereNothingSaysOtherwise(t *testing.T) {
	moved := 0
	for _, tc := range fourWordRows {
		out, st := run(t, tc.src, negateOnce(No))
		if st != tc.twice {
			t.Errorf("%s = %d, want %d (stdout %q)", tc.src, st, tc.twice, out)
		}
		if tc.once != tc.twice {
			moved++
		}
		if tc.control && tc.once != tc.twice {
			t.Errorf("%s is marked a control and moves with the axis", tc.src)
		}
	}
	// A floor rather than the figure: a row the two readings happen to agree
	// on is still worth keeping above, and the table stops measuring the axis
	// the moment nothing moves with it.
	if moved < 7 {
		t.Errorf("%d of %d rows moved with the axis, want at least 7", moved, len(fourWordRows))
	}
}

// And the folding is decided by what the three words behind the `!` are read
// as, not by their spelling — which is the probe that separates this from
// "two leading `!`s cancel".
//
// Both lists begin `! !` and hold four words. One is negated once and the
// other twice, and the difference is entirely in whether the second `!` is a
// negation or the left operand of a comparison.
func TestTheFoldingFollowsTheReadingAndNotTheLeadingWords(t *testing.T) {
	if _, st := run(t, `test ! ! -n x`, negateOnce(Yes)); st != 1 {
		t.Errorf("a negation behind the `!`: status %d, want 1 — the outer `!` is absorbed", st)
	}
	if _, st := run(t, `test ! ! = x`, negateOnce(Yes)); st != 0 {
		t.Errorf("a comparison behind the `!`: status %d, want 0 — the outer `!` negates", st)
	}
	// The three-word readings those two fold onto, which is where the
	// statuses above come from.
	if _, st := run(t, `test ! -n x`, negateOnce(Yes)); st != 1 {
		t.Errorf("`test ! -n x`: status %d, want 1", st)
	}
	if _, st := run(t, `test ! = x`, negateOnce(Yes)); st != 1 {
		t.Errorf("`test ! = x`: status %d, want 1 — a comparison of two strings", st)
	}
}

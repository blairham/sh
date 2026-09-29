// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// Semantics.TestNegationBeforeAnOperatorIsAnOperand: a `!` standing where a
// primary belongs and followed by an **operator** is the string `!` rather
// than a negation.
//
// Graded on the status alone, which is the whole of the answer — every row
// exits without output, and the readings part between a value and a refusal.
//
// The `operand` column is zsh 5.9.2's measurement; see the axis for where it
// was taken and for the six columns beside it.

func negationIsAnOperand(yes Answer) func(*Runner) {
	return func(r *Runner) {
		s := *r.Semantics
		s.TestNegationBeforeAnOperatorIsAnOperand = yes
		r.Semantics = &s
	}
}

// negationOperandRows are the five-and-more word shapes, with the status each
// reading gives. A control is a row the two readings answer alike, which is
// what says the axis is not "a `!` stops negating".
var negationOperandRows = []struct {
	src              string
	operand, negated int
	control          bool
}{
	// The `!`s that are operands: a connective follows, so there is nothing
	// for a negation to take.
	{src: `test ! -a ! -a !`, operand: 0, negated: 2},
	{src: `test ! -o ! -o !`, operand: 0, negated: 2},
	{src: `test ! -a x -a y`, operand: 0, negated: 2},
	{src: `test ! -a "" -a y`, operand: 1, negated: 2},
	{src: `test x -a ! -a y`, operand: 0, negated: 2},
	{src: `test x -a ! -a ! -a y`, operand: 0, negated: 2},
	{src: `test ! -a ! -a ! -a !`, operand: 0, negated: 2},
	// A binary operator follows, so the `!` is its left operand.
	{src: `test ! = ! -a x`, operand: 0, negated: 2},
	{src: `test ! != y -a x`, operand: 0, negated: 2},
	{src: `test x -a ! = y`, operand: 1, negated: 2},
	// One `!` negating another that is an operand, which is what keeps the
	// rule from being "a leading `!` is always a string".
	{src: `test ! ! -a ! -a !`, operand: 1, negated: 2},

	// The controls. An ordinary word follows, so both readings negate.
	{src: `test ! x -a y`, operand: 1, negated: 1, control: true},
	// `!` in front of an ordinary word negates, and the empty string is
	// one: `!("" -a y)` is true because `""` is false.
	{src: `test ! "" -a y`, operand: 0, negated: 0, control: true},
	{src: `test ! x = y -a z`, operand: 0, negated: 0, control: true},
	// A **unary** operator follows, which is the row that says the
	// lookahead is about operators a `!` cannot stand in front of and not
	// about every word spelled with a dash.
	{src: `test ! -n x -a y`, operand: 1, negated: 1, control: true},
	{src: `test ! -z "" -a y`, operand: 1, negated: 1, control: true},
	{src: `test x -a ! -n y`, operand: 1, negated: 1, control: true},
	// A `!` taken as a binary operator's **right** operand never reaches
	// this question: the binary parse has it before a primary is read.
	{src: `test x = ! -a y`, operand: 1, negated: 1, control: true},
	// And the counts below the grammar, which the fixed-arity readings
	// answer and this axis does not touch. Three words are deliberately not
	// here: `test ! -a !` is Semantics.TestThreeWordsNegateBeforeAConnective
	// and refuses in the core, so it would be a row about that axis.
	{src: `test ! x`, operand: 1, negated: 1, control: true},
	{src: `test ! !`, operand: 1, negated: 1, control: true},
}

func TestANegationBeforeAnOperatorIsReadAsTheStringItIsSpelledWith(t *testing.T) {
	for _, tc := range negationOperandRows {
		t.Run(tc.src, func(t *testing.T) {
			out, st := run(t, tc.src, negationIsAnOperand(Yes))
			if st != tc.operand {
				t.Errorf("%s = %d, want %d (stdout %q)", tc.src, st, tc.operand, out)
			}
		})
	}
}

// The other answer over the same rows, which is the mutation: the eleven that
// are not controls move and the twelve controls do not.
func TestANegationBeforeAnOperatorNegatesWhereNothingSaysOtherwise(t *testing.T) {
	moved := 0
	for _, tc := range negationOperandRows {
		out, st := run(t, tc.src, negationIsAnOperand(No))
		if st != tc.negated {
			t.Errorf("%s = %d, want %d (stdout %q)", tc.src, st, tc.negated, out)
		}
		if tc.control && tc.operand != tc.negated {
			t.Errorf("%s is marked a control and the two readings differ", tc.src)
		}
		if !tc.control {
			if tc.operand == tc.negated {
				t.Errorf("%s is not marked a control and the two readings agree", tc.src)
			}
			moved++
		}
	}
	// A count rather than a spot check: a lookahead that stopped firing
	// would leave every row on the negation reading and this test would
	// still pass row by row.
	if moved == 0 {
		t.Error("no row distinguishes the two readings, so neither was measured")
	}
}

// Unanswered, the axis refuses by name — and only where the two readings
// part. A `!` in front of an ordinary word or a unary operator is unanimous
// across the panel, so asking there would report an unanswered axis over a
// question nobody disagrees about.
func TestTheNegationOperandAxisIsAskedOnlyWhereTheColumnsPart(t *testing.T) {
	for _, tc := range negationOperandRows {
		out, _ := run(t, tc.src, negationIsAnOperand(Unspecified))
		refused := containsRefusal(out)
		if refused != !tc.control {
			verb := map[bool]string{true: "refused", false: "answered"}[refused]
			t.Errorf("%s %s with the axis unanswered; control=%v", tc.src, verb, tc.control)
		}
	}
}

func containsRefusal(out string) bool {
	return len(out) > 0 && (indexOf(out, "the shells disagree here") >= 0 ||
		indexOf(out, "no dialect was chosen") >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

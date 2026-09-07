// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/syntax"
)

// A byte the dialect's arithmetic reader refuses as part of no token gets a
// verdict of its own — but only where the expression could legally have
// stopped. Names the flag and not a shell, which is the rule for a test here:
// one shell in the panel fills the table in and three leave it empty, and
// nothing below asks which is which.

// refusing is a dialect whose arithmetic reader refuses `@ { } ;`, which is
// the shape of a real one's table without being a claim about any shell's.
func refusing() syntax.Dialect {
	d := syntax.Core()
	d.ArithBytesRefusedOutright = "@{};"
	return d
}

func arithKind(t *testing.T, expr string, d syntax.Dialect) *syntax.Error {
	t.Helper()
	err := arithErr(expr, d)
	var se *syntax.Error
	if !errors.As(err, &se) {
		t.Fatalf("%s: got %v, want a refusal", expr, err)
	}
	return se
}

// TestARefusedByteAnswersForItselfWhereTheExpressionCouldHaveStopped is the
// positional rule, which is the whole of this feature: the byte alone does
// not decide.
func TestARefusedByteAnswersForItselfWhereTheExpressionCouldHaveStopped(t *testing.T) {
	for _, tc := range []struct {
		expr  string
		kind  syntax.ErrorKind
		token string
		why   string
	}{
		// Nothing read yet: there is no operator to have been left wanting a
		// value, so the byte answers for itself.
		{`@`, syntax.ErrArithIllegalByte, "@", "at the start"},
		{`  @  `, syntax.ErrArithIllegalByte, "@", "whitespace is not reading"},
		{`@1`, syntax.ErrArithIllegalByte, "@", "text after it changes nothing"},
		{`{`, syntax.ErrArithIllegalByte, "{", "another byte in the table"},
		{`;`, syntax.ErrArithIllegalByte, ";", "and another"},
		// Where an operator belonged: a complete value has been read and the
		// expression could have stopped, so again the byte answers.
		{`1 @`, syntax.ErrArithIllegalByte, "@", "after a number"},
		{`1@`, syntax.ErrArithIllegalByte, "@", "with no space"},
		{`a@`, syntax.ErrArithIllegalByte, "@", "after a name"},
		{`(1) @`, syntax.ErrArithIllegalByte, "@", "after a group"},
		{`1+2 @`, syntax.ErrArithIllegalByte, "@", "after an expression"},
		{`1 @@`, syntax.ErrArithIllegalByte, "@", "the first byte, not the run"},
		// Where an operand was wanted: an operator has just been consumed,
		// so the failure is the operator's and the text blamed is the rest.
		{`1+@`, syntax.ErrArithOperand, "@", "after a binary operator"},
		{`+@`, syntax.ErrArithOperand, "@", "after a unary one"},
		{`~@`, syntax.ErrArithOperand, "@", "after another unary one"},
		{`1*@`, syntax.ErrArithOperand, "@", "after any binary one"},
		{`1?2:@`, syntax.ErrArithOperand, "@", "after a conditional's colon"},
		{`1+@2`, syntax.ErrArithOperand, "@2", "and it names the rest, not the byte"},
	} {
		se := arithKind(t, tc.expr, refusing())
		if se.Kind != tc.kind {
			t.Errorf("%s (%s): kind %v, want %v", tc.expr, tc.why, se.Kind, tc.kind)
		}
		if se.Token != tc.token {
			t.Errorf("%s (%s): token %q, want %q", tc.expr, tc.why, se.Token, tc.token)
		}
	}
}

// TestAnEmptyTableChangesNothing. Three of the panel have no such sentence,
// and for them this must be invisible: the same expressions keep exactly the
// kinds they had before the table existed.
func TestAnEmptyTableChangesNothing(t *testing.T) {
	if got := syntax.Core().ArithBytesRefusedOutright; got != "" {
		t.Fatalf("the core names %q; a dialect with no such sentence must name nothing", got)
	}
	for _, tc := range []struct {
		expr string
		kind syntax.ErrorKind
	}{
		{`@`, syntax.ErrArithOperand},
		{`1 @`, syntax.ErrArithBadOperator},
		{`1+@`, syntax.ErrArithOperand},
		{`{`, syntax.ErrArithOperand},
		{`1 ;`, syntax.ErrArithBadOperator},
	} {
		if se := arithKind(t, tc.expr, syntax.Core()); se.Kind != tc.kind {
			t.Errorf("%s with an empty table: kind %v, want %v", tc.expr, se.Kind, tc.kind)
		}
	}
}

// TestAByteOutsideTheTableIsUnaffected. The table is a table: a byte not in
// it keeps the reading it had, in the same dialect that refuses others.
func TestAByteOutsideTheTableIsUnaffected(t *testing.T) {
	for _, tc := range []struct {
		expr string
		kind syntax.ErrorKind
	}{
		// `%` is a math token, so it is an operator wanting a value.
		{`%`, syntax.ErrArithOperand},
		{`1 %`, syntax.ErrArithOperandEnd},
		// A value where an operator belonged is still that.
		{`1 2`, syntax.ErrArithOperator},
		// And text that could be neither, but is not in the table.
		{`1.5`, syntax.ErrArithBadOperator},
	} {
		if se := arithKind(t, tc.expr, refusing()); se.Kind != tc.kind {
			t.Errorf("%s: kind %v, want %v", tc.expr, se.Kind, tc.kind)
		}
	}
}

// TestARefusedByteDoesNotBreakWhatReads. The table is only consulted where a
// read has already failed, so an expression that reads cleanly must be
// untouched by it — including the two shapes whose own syntax uses a byte
// near the table's.
func TestARefusedByteDoesNotBreakWhatReads(t *testing.T) {
	d := refusing()
	d.ArithCharacterCode = true
	d.ArraySubscript = true
	for _, expr := range []string{`2+3`, `##a`, `a[1]`, `a[1]+1`, `(1+2)*3`, `1?2:3`} {
		if err := arithErr(expr, d); err != nil {
			t.Errorf("%s: %v, want a clean read", expr, err)
		}
	}
}

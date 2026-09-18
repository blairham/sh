// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/blairham/sh/syntax"
)

// arithShape renders a parsed expression as a prefix string, which is how a
// precedence question is stated without a second parser to compare against.
func arithShape(t *testing.T, src string, d syntax.Dialect) string {
	t.Helper()
	return shapeOf(arithOf(t, src, d))
}

func shapeOf(x syntax.ArithExpr) string {
	switch n := x.(type) {
	case *syntax.ArithBinary:
		return "(" + n.Op + " " + shapeOf(n.X) + " " + shapeOf(n.Y) + ")"
	case *syntax.ArithNum:
		return n.Text
	case *syntax.ArithVar:
		return n.Name
	}
	return fmt.Sprintf("%T", x)
}

// logicalXor is the core plus the logical exclusive-or. The flag is named
// here and the shell that sets it is not.
func logicalXor(on bool, order syntax.ArithPrecedencePolicy) syntax.Dialect {
	d := syntax.Core()
	d.ArithLogicalXor = on
	d.ArithPrecedence = order
	return d
}

// Where the operator sits on each of the two ladders. Both are measured; see
// docs/spec/grammar/arithmetic.md for the probes that separate the readings a
// simpler table would have allowed.
func TestTheLogicalXorSitsOnEachLadder(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		src    string
		own    string // the dialect's own order
		cOrder string
	}{
		// On its own ladder `^^` shares the `||` rung, left-associative; in
		// C's order it takes a rung between `||` and `&&`.
		{"1 || 0 ^^ 1", "(^^ (|| 1 0) 1)", "(|| 1 (^^ 0 1))"},
		{"1 ^^ 1 || 1", "(|| (^^ 1 1) 1)", "(|| (^^ 1 1) 1)"},
		{"1 || 1 ^^ 1", "(^^ (|| 1 1) 1)", "(|| 1 (^^ 1 1))"},
		// `&&` binds tighter than `^^` on both.
		{"1 ^^ 0 && 0", "(^^ 1 (&& 0 0))", "(^^ 1 (&& 0 0))"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			for _, ladder := range []struct {
				name  string
				order syntax.ArithPrecedencePolicy
				want  string
			}{
				{"its own order", syntax.ArithPrecedenceShiftsAndBitwiseBindTighter, tc.own},
				{"C's order", syntax.ArithPrecedenceAsInC, tc.cOrder},
			} {
				got := arithShape(t, "echo $(( "+tc.src+" ))", logicalXor(true, ladder.order))
				if got != ladder.want {
					t.Errorf("%s: shape %q, want %q", ladder.name, got, ladder.want)
				}
			}
		})
	}
}

// With the flag off the spelling is already taken: `^` is bitwise xor, so the
// doubled character is an xor whose right operand is missing — which is what
// every column without the operator reports, and what makes this a flag
// rather than an unconditional grammar.
func TestWithoutTheFlagTheDoubledCaretIsABitwiseXorThatRanOut(t *testing.T) {
	t.Parallel()
	err := arithErr(`1 ^^ 1`, logicalXor(false, syntax.ArithPrecedenceAsInC))
	var se *syntax.Error
	if !errors.As(err, &se) {
		t.Fatalf("read `1 ^^ 1` without the flag: %v, want a syntax.Error", err)
	}
	if se.Kind != syntax.ErrArithOperand {
		t.Errorf("kind %v, want %v", se.Kind, syntax.ErrArithOperand)
	}
	if se.Token != "^ 1" {
		t.Errorf("token %q, want %q — the second caret is where the operand was wanted", se.Token, "^ 1")
	}
	// And with the flag on the same text reads, which is the other half of
	// the flag being load-bearing.
	if err := arithErr(`1 ^^ 1`, logicalXor(true, syntax.ArithPrecedenceAsInC)); err != nil {
		t.Errorf("read `1 ^^ 1` with the flag: %v, want it to parse", err)
	}
	// The bitwise operator is untouched either way.
	if err := arithErr(`1 ^ 1`, logicalXor(true, syntax.ArithPrecedenceAsInC)); err != nil {
		t.Errorf("read `1 ^ 1` with the flag: %v, want it to parse", err)
	}
}

// The assignment spelling is an assignment operator and not a rung op where
// the left side is a name: it is looser than `||`, so the whole of what
// follows is its value.
func TestTheLogicalXorAssignmentBindsAsAnAssignment(t *testing.T) {
	t.Parallel()
	d := logicalXor(true, syntax.ArithPrecedenceShiftsAndBitwiseBindTighter)
	x, ok := arithOf(t, `echo $(( x ^^= 1 || 1 ))`, d).(*syntax.ArithAssign)
	if !ok {
		t.Fatalf("not an assignment: %T", arithOf(t, `echo $(( x ^^= 1 || 1 ))`, d))
	}
	if x.Op != "^^=" {
		t.Errorf("operator %q, want %q", x.Op, "^^=")
	}
	if b, ok := x.Value.(*syntax.ArithBinary); !ok || b.Op != "||" {
		t.Errorf("value %#v, want the whole `1 || 1`", x.Value)
	}
}

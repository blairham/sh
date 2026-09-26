// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// exponentAssign is the core plus `**=`. The flag is named here and the shell
// that sets it is not.
//
// [syntax.Core] already has `**`, which is the whole point of the flag being
// separate: every column that parses the operator at all was asked, and three
// of the four refuse the assignment spelling. So `on` moves one thing and the
// exponent operator is present on both sides of every row below.
func exponentAssign(on bool) syntax.Dialect {
	d := syntax.Core()
	d.ArithExponentAssign = on
	return d
}

// With the flag on, `x **= 3` is an assignment whose operator is the whole of
// `**=`.
func TestTheExponentAssignmentIsOneOperator(t *testing.T) {
	t.Parallel()
	src := `echo $(( x **= 3 ))`
	x, ok := arithOf(t, src, exponentAssign(true)).(*syntax.ArithAssign)
	if !ok {
		t.Fatalf("parse %q: %T, want an assignment", src, arithOf(t, src, exponentAssign(true)))
	}
	if x.Op != "**=" {
		t.Errorf("operator %q, want %q", x.Op, "**=")
	}
	if x.Name != "x" {
		t.Errorf("target %q, want %q", x.Name, "x")
	}
	if n, ok := x.Value.(*syntax.ArithNum); !ok || n.Text != "3" {
		t.Errorf("value %#v, want the number 3", x.Value)
	}
}

// And it is right-associative and looser than every binary rung, which is
// what makes it an assignment rather than a rung op: the whole of what
// follows is its value.
func TestTheExponentAssignmentBindsAsAnAssignment(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, want string }{
		{"the whole of a binary expression is the value", `a **= 1 + 2`, "(+ 1 2)"},
		{"and a second assignment is read to the right", `a **= b **= 2`, "(b **= 2)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x, ok := arithOf(t, `echo $(( `+tc.src+` ))`, exponentAssign(true)).(*syntax.ArithAssign)
			if !ok {
				t.Fatalf("parse %q: not an assignment", tc.src)
			}
			if got := assignShape(x.Value); got != tc.want {
				t.Errorf("value %s, want %s", got, tc.want)
			}
		})
	}
}

// assignShape is [shapeOf] with the assignment node added, so a nested
// assignment can be stated in one string.
func assignShape(x syntax.ArithExpr) string {
	if a, ok := x.(*syntax.ArithAssign); ok {
		return "(" + a.Name + " " + a.Op + " " + assignShape(a.Value) + ")"
	}
	return shapeOf(x)
}

// The flag is load-bearing in both directions, and the row that says so is
// the same text read twice.
//
// With the flag off the spelling is not taken by anything else — which is
// where this parts from `^^=`, whose characters are already a bitwise xor. It
// is the exponent operator with its right operand missing, so the refusal is
// about the operand and names the `=` that stood where one was wanted.
func TestWithoutTheFlagTheExponentAssignmentIsAnOperandThatRanOut(t *testing.T) {
	t.Parallel()
	err := arithErr(`x **= 3`, exponentAssign(false))
	var se *syntax.Error
	if !errors.As(err, &se) {
		t.Fatalf("read `x **= 3` without the flag: %v, want a syntax.Error", err)
	}
	if se.Kind != syntax.ErrArithOperand {
		t.Errorf("kind %v, want %v", se.Kind, syntax.ErrArithOperand)
	}
	if se.Token != "= 3" {
		t.Errorf("token %q, want %q — the `=` is where the exponent's operand was wanted", se.Token, "= 3")
	}
	if err := arithErr(`x **= 3`, exponentAssign(true)); err != nil {
		t.Errorf("read `x **= 3` with the flag: %v, want it to parse", err)
	}
}

// `**` is untouched by the flag, which is the other half of the two being
// separate questions. Both shapes are asserted on both settings.
func TestTheExponentOperatorIsTheSameEitherWay(t *testing.T) {
	t.Parallel()
	for _, on := range []bool{false, true} {
		for _, tc := range []struct{ src, want string }{
			// Right-associative, and the unary sign is part of the base.
			{"2 ** 3 ** 2", "(** 2 (** 3 2))"},
			{"2 * 3 ** 2", "(* 2 (** 3 2))"},
			{"a ** b", "(** a b)"},
		} {
			if got := arithShape(t, "echo $(( "+tc.src+" ))", exponentAssign(on)); got != tc.want {
				t.Errorf("flag %v: %q is %s, want %s", on, tc.src, got, tc.want)
			}
		}
	}
}

// An exponent assignment to something that cannot hold a value is refused as
// an assignment to a non-place, and not as the exponent operator running out
// of an operand.
//
// The whole operator is blamed rather than the `=` the ladder would have left
// behind, which is the same shape every other compound spelling already has —
// see the `+=` row beside it, which needs no flag.
func TestAnExponentAssignmentToANonPlaceIsRefusedAsOne(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src string }{
		{"a number on the left", `1 **= 2`},
		{"a name buried in an expression", `1 + x **= 3`},
		{"a group on the left", `(x) **= 3`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := arithErr(tc.src, exponentAssign(true))
			var se *syntax.Error
			if !errors.As(err, &se) {
				t.Fatalf("read %q: %v, want a syntax.Error", tc.src, err)
			}
			if se.Kind != syntax.ErrArithAssignToNonPlace {
				t.Errorf("kind %v, want %v", se.Kind, syntax.ErrArithAssignToNonPlace)
			}
			// And the *whole* operator is blamed, not the `=` the ladder
			// would have left: without the guard in arithParser.power the
			// exponent rung takes the `**` first and the complaint is about
			// a missing operand instead.
			if !strings.HasPrefix(se.Token, "**=") {
				t.Errorf("token %q, want it to begin with the whole operator", se.Token)
			}
		})
	}
	// Its control on the operator that has never needed a flag, so the row
	// above is about `**=` reaching the same path and not about the path.
	err := arithErr(`1 += 2`, exponentAssign(true))
	var se *syntax.Error
	if !errors.As(err, &se) || se.Kind != syntax.ErrArithAssignToNonPlace {
		t.Errorf("`1 += 2`: %v, want %v", err, syntax.ErrArithAssignToNonPlace)
	}
}

// Three characters and not two beside an `=`, which the flag could otherwise
// have loosened: a space between them is the exponent operator with a missing
// operand, and a third `*` is the same.
func TestTheExponentAssignmentIsNotSpeltApart(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, token string }{
		{`x ** = 3`, "= 3"},
		{`x ***= 3`, "*= 3"},
	} {
		err := arithErr(tc.src, exponentAssign(true))
		var se *syntax.Error
		if !errors.As(err, &se) {
			t.Fatalf("read %q: %v, want a syntax.Error", tc.src, err)
		}
		if se.Kind != syntax.ErrArithOperand {
			t.Errorf("%q: kind %v, want %v", tc.src, se.Kind, syntax.ErrArithOperand)
		}
		// The blame is where the operand was wanted, which is the reference
		// shell's own: measured 2026-09-26 on zsh 5.9.2, `$(( x ***= 3 ))` is
		// ``operand expected at `*= 3 '``.
		if se.Token != tc.token {
			t.Errorf("%q: token %q, want %q", tc.src, se.Token, tc.token)
		}
	}
}

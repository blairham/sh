// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// A point that begins a refused token is a *third* refusal in the dialect
// with floats, alongside "operand expected" and "operator expected".
//
// Measured 2026-09-11 and 2026-09-12 on zsh 5.9.2: `$(( .foo ))`, `$(( 1..2 ))`
// and `$(( . ))` are all `bad floating point constant`, where the same shell
// answers `$(( 1 % ))` with `bad math expression: operand expected …`. So the
// point commits the reader to a floating literal and failing to read one is
// its own sentence rather than a wording of either (#1889).
//
// It is a value one dialect holds and not an axis: ksh93 has floats too and
// answers the same expressions with its ordinary operand complaints, so an
// empty field leaves those untouched.
func TestAPointThatBeginsARefusedTokenIsAFloatConstant(t *testing.T) {
	d := Diagnostics{
		ArithError:            "%[1]s: %[2]s",
		ArithOperandExpected:  "operand expected at `%[1]s'",
		ArithOperatorExpected: "operator expected at `%[1]s'",
		ArithExpressionRanOut: "operand expected at end of string",
		ArithBadFloatConstant: "bad floating point constant",
	}
	dial := syntax.Core()
	dial.ArithFloat = true
	for _, tc := range []struct{ src, want string }{
		// The point begins the token, in the operand position and in the
		// operator position alike.
		{".foo", "bad floating point constant"},
		{"1 + .foo", "bad floating point constant"},
		{"1..2", "bad floating point constant"},
		{".", "bad floating point constant"},
		{"1 . 2", "bad floating point constant"},
		{"a.b", "bad floating point constant"},
		{".5.5", "bad floating point constant"},
		// And the control that says the *point* decides rather than the
		// float reader having been entered at all: `1.` reads as a float and
		// the token left over is `e`, which gets the ordinary sentence.
		{"1.e", "1.e: operator expected at `e'"},
		{"1 %", "1 %: operand expected at end of string"},
	} {
		err := arithErr(tc.src, dial)
		if err == nil {
			t.Fatalf("%s: read cleanly, want a refusal", tc.src)
		}
		if got := d.ParseFailure(err); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.src, got, tc.want)
		}
	}
	// `.5` is a float and not a refusal at all, which is what says the rule
	// is about a point the reader could not finish.
	if err := arithErr(".5", dial); err != nil {
		t.Errorf(".5: %v, want a number", err)
	}
}

// A dialect with no such sentence keeps its operand complaints, which is the
// half that makes this a value rather than an axis.
func TestADialectWithoutTheSentenceKeepsItsOperandComplaint(t *testing.T) {
	d := Diagnostics{
		ArithError:           "%[1]s: %[2]s",
		ArithOperandExpected: "operand expected at `%[1]s'",
	}
	dial := syntax.Core()
	dial.ArithFloat = true
	if got, want := d.ParseFailure(arithErr(".foo", dial)), ".foo: operand expected at `.foo'"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

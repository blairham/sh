// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// mathCall is the core plus the math-function call form. The flag is named
// here and the shell that sets it is not.
func mathCall(on bool) syntax.Dialect {
	d := syntax.Core()
	d.ArithFunctionCall = on
	return d
}

// TestAMathFunctionCallReadsItsArguments — the shape of the node, which is
// what the evaluator dispatches on.
func TestAMathFunctionCallReadsItsArguments(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		fn   string
		args int
		text string
	}{
		{"no arguments", "echo $(( mf() ))", "mf", 0, "mf()"},
		{"one", "echo $(( mf(5) ))", "mf", 1, "mf(5)"},
		{"two", "echo $(( mf(5,6) ))", "mf", 2, "mf(5,6)"},
		// The comma inside the parentheses separates arguments; it is not
		// the sequence operator, which the same dialect may also have.
		{"expressions", "echo $(( mf(1+1,2*3) ))", "mf", 2, "mf(1+1,2*3)"},
		{"spaces kept in the text", "echo $(( mf( 5 , 6 ) ))", "mf", 2, "mf( 5 , 6 )"},
		{"a name argument", "echo $(( mf(x) ))", "mf", 1, "mf(x)"},
		{"a nested call", "echo $(( mf(mf(1)) ))", "mf", 1, "mf(mf(1))"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x, ok := arithOf(t, tc.src, mathCall(true)).(*syntax.ArithCall)
			if !ok {
				t.Fatalf("parse %q: not a call node", tc.src)
			}
			if x.Name != tc.fn || len(x.Args) != tc.args || x.Text != tc.text {
				t.Errorf("parse %q: name %q args %d text %q, want %q %d %q",
					tc.src, x.Name, len(x.Args), x.Text, tc.fn, tc.args, tc.text)
			}
		})
	}
}

// TestACallComposesWithTheRestOfAnExpression — an operand like any other,
// which is what makes it a node in the tree rather than a statement about the
// whole expression.
func TestACallComposesWithTheRestOfAnExpression(t *testing.T) {
	x, ok := arithOf(t, "echo $(( mf(5) + 1 ))", mathCall(true)).(*syntax.ArithBinary)
	if !ok {
		t.Fatalf("parse: not a binary node")
	}
	if _, ok := x.X.(*syntax.ArithCall); !ok {
		t.Errorf("left operand is %T, want a call", x.X)
	}
}

// TestTheParenthesisHasToTouchTheName. A space between them is not a call in
// the shell that has one either, so the flag never changes what `a (b)` means
// — which is the half that makes the construct additive rather than a second
// reading of text the other dialects already accept.
func TestTheParenthesisHasToTouchTheName(t *testing.T) {
	if _, ok := arithOf(t, "echo $(( mf ))", mathCall(true)).(*syntax.ArithVar); !ok {
		t.Errorf("a bare name is not a variable reference")
	}
	if err := arithErr("mf ( 5 )", mathCall(true)); err == nil {
		t.Errorf("a space before the parenthesis was read, want a refusal")
	}
}

// TestWithoutTheFlagACallIsALeftoverParenthesis, which is what the five shells
// without the construct report — the grammar's absence rather than a different
// meaning for the same text.
func TestWithoutTheFlagACallIsALeftoverParenthesis(t *testing.T) {
	if err := arithErr("mf(5)", mathCall(false)); err == nil {
		t.Errorf("a call was read in a dialect without the form, want a refusal")
	}
	// And the flag does not disturb an ordinary parenthesised sub-expression,
	// which begins where no name has been read.
	for _, on := range []bool{true, false} {
		if err := arithErr("(1+2)*3", mathCall(on)); err != nil {
			t.Errorf("flag %v: a parenthesised sub-expression: %v", on, err)
		}
	}
}

// TestAnUnclosedCallIsRefused rather than read to the end of the expression.
func TestAnUnclosedCallIsRefused(t *testing.T) {
	for _, expr := range []string{"mf(5", "mf(5 6)", "mf(,)", "mf(5,,6)"} {
		if err := arithErr(expr, mathCall(true)); err == nil {
			t.Errorf("$((%s)) was read, want a refusal", expr)
		}
	}
	// A trailing comma before the `)` is not one of them: measured, `mf(5,)`
	// passes one argument and `mf(5,6,)` two, so it is allowed and counts
	// nothing.
	for _, tc := range []struct {
		expr string
		args int
	}{{"mf(5,)", 1}, {"mf(5,6,)", 2}, {"mf( )", 0}} {
		x, ok := arithOf(t, "echo $(( "+tc.expr+" ))", mathCall(true)).(*syntax.ArithCall)
		if !ok {
			t.Fatalf("parse %q: not a call node", tc.expr)
		}
		if len(x.Args) != tc.args {
			t.Errorf("parse %q: %d arguments, want %d", tc.expr, len(x.Args), tc.args)
		}
	}
}

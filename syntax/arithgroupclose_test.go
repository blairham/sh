// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/syntax"
)

// A parenthesised group that reads a value and then meets text it can use for
// neither an operator nor a close is an *arithmetic* failure and not a parse
// one, and it is its own kind because two dialects have a sentence for it
// that they give no other leftover.
//
// It used to be this parser's own prose about a production — `expected ) in
// arithmetic` — which meant the text was refused before the expression could
// be quoted back, and a dialect that ends a script over a parse failure ended
// it at the wrong status (#3071).
func TestAnUnclosedGroupIsAnArithmeticFailure(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		src   string
		kind  syntax.ErrorKind
		token string
	}{
		{`(echo a)`, syntax.ErrArithMissingCloseParen, "a)"},
		{`(1 2`, syntax.ErrArithMissingCloseParen, "2"},
		{`(1 + 2 3)`, syntax.ErrArithMissingCloseParen, "3)"},
		// The neighbors, which reach other kinds and always did: a group
		// that closes and is then followed by leftover text, and one whose
		// contents are not a value at all.
		{`(1) 2`, syntax.ErrArithOperator, "2"},
		{`()`, syntax.ErrArithOperand, ")"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			err := arithErr(tc.src, syntax.Core())
			var se *syntax.Error
			if !errors.As(err, &se) {
				t.Fatalf("read %q: %v, want a syntax.Error", tc.src, err)
			}
			if se.Kind != tc.kind {
				t.Errorf("read %q: kind %v, want %v", tc.src, se.Kind, tc.kind)
			}
			if se.Expr != tc.src {
				t.Errorf("read %q: expression %q, want the whole of it", tc.src, se.Expr)
			}
			if se.Token != tc.token {
				t.Errorf("read %q: token %q, want %q", tc.src, se.Token, tc.token)
			}
		})
	}
}

// Whether `++` and `--` are operators is Dialect.ArithIncDec, and where they
// are not the doubled sign is not refused: it is two unary signs, which is
// what the one panel column without the operators evaluates.
func TestWithoutTheIncrementOperatorsADoubledSignIsTwoSigns(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.ArithIncDec = false
	if err := arithErr(`--x`, d); err != nil {
		t.Errorf("read %q without the operators: %v, want two signs", "--x", err)
	}
	x, ok := arithOf(t, `echo $(( --x ))`, d).(*syntax.ArithUnary)
	if !ok {
		t.Fatalf("read %q: not a unary expression", "--x")
	}
	if x.Op != "-" {
		t.Errorf("outer operator %q, want %q", x.Op, "-")
	}
	inner, ok := x.X.(*syntax.ArithUnary)
	if !ok || inner.Op != "-" {
		t.Errorf("inner %#v, want a second `-`", x.X)
	}
	// And with the operators it is one token again, which is what makes the
	// flag load-bearing rather than decoration.
	d.ArithIncDec = true
	y, ok := arithOf(t, `echo $(( --x ))`, d).(*syntax.ArithUnary)
	if !ok || y.Op != "--" {
		t.Fatalf("read %q with the operators: %#v, want one `--`", "--x", y)
	}
}

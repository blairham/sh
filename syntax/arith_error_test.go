// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/syntax"
)

// arithErr reads one expression as this dialect and returns what the read had
// to say — the route the interpreter takes when the command runs, which is
// the only place a bad expression is refused now (#865).
func arithErr(expr string, d syntax.Dialect) error {
	p := syntax.NewParser("", d)
	p.ParseArithFor(expr, syntax.Pos{})
	return p.Err()
}

// A failure inside `$(( ))` is an arithmetic failure and not a syntax error.
//
// Every shell in the panel reports it the way it reports a division by zero —
// the expression quoted, and a reason — rather than the way it reports a stray
// `)`. It carries the whole expression and the part blamed for it, because one
// shell quotes the first and another names the second.
func TestArithmeticFailureIsItsOwnKind(t *testing.T) {
	for _, tc := range []struct {
		src   string
		kind  syntax.ErrorKind
		expr  string
		token string
	}{
		{`1 2`, syntax.ErrArithOperator, "1 2", "2"},
		{`a b`, syntax.ErrArithOperator, "a b", "b"},
		{`1+`, syntax.ErrArithOperandEnd, "1+", "+"},
		{`1*`, syntax.ErrArithOperandEnd, "1*", "*"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			err := arithErr(tc.src, syntax.Core())
			var se *syntax.Error
			if !errors.As(err, &se) {
				t.Fatalf("got %v, want a *syntax.Error", err)
			}
			if se.Kind != tc.kind {
				t.Errorf("kind = %v, want %v", se.Kind, tc.kind)
			}
			if se.Expr != tc.expr {
				t.Errorf("expr = %q, want %q", se.Expr, tc.expr)
			}
			if se.Token != tc.token {
				t.Errorf("token = %q, want %q", se.Token, tc.token)
			}
		})
	}
}

// The two ways an operand can be missing are two kinds, because two of the
// panel word them apart and the parser is the only place they can be told
// apart: running out leaves the frame that wanted the operand holding the
// operator, and finding something leaves text nothing can begin a value with.
//
// The token differs with the kind and that is deliberate. What ran out is
// named by the operator left wanting; what was found is named by the text from
// the refused byte to the end of the expression, which is what the shells that
// name anything here name.
func TestAnOperandMissingIsTwoKinds(t *testing.T) {
	for _, tc := range []struct {
		src   string
		kind  syntax.ErrorKind
		token string
	}{
		{`1+`, syntax.ErrArithOperandEnd, "+"},
		{`~`, syntax.ErrArithOperandEnd, "~"},
		{`!`, syntax.ErrArithOperandEnd, "!"},
		{`1**`, syntax.ErrArithOperandEnd, "**"},
		{`a=`, syntax.ErrArithOperandEnd, "="},
		{`%`, syntax.ErrArithOperand, "%"},
		{`@`, syntax.ErrArithOperand, "@"},
		{`1+&2`, syntax.ErrArithOperand, "&2"},
		{`()`, syntax.ErrArithOperand, ")"},
		{`1+*`, syntax.ErrArithOperand, "*"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			err := arithErr(tc.src, syntax.Core())
			var se *syntax.Error
			if !errors.As(err, &se) {
				t.Fatalf("got %v, want a *syntax.Error", err)
			}
			if se.Kind != tc.kind {
				t.Errorf("kind = %v, want %v", se.Kind, tc.kind)
			}
			if se.Token != tc.token {
				t.Errorf("token = %q, want %q", se.Token, tc.token)
			}
		})
	}
}

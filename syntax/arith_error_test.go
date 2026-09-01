// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/syntax"
)

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
		{`$((1 2))`, syntax.ErrArithOperator, "1 2", "2"},
		{`$((a b))`, syntax.ErrArithOperator, "a b", "b"},
		{`$((1+))`, syntax.ErrArithOperand, "1+", "+"},
		{`$((1*))`, syntax.ErrArithOperand, "1*", "*"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := syntax.Parse("echo "+tc.src, syntax.Core())
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

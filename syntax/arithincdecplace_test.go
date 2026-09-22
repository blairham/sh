// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/syntax"
)

// Dialect.ArithIncDecNeedsAPlace is whether `++` and `--` are operators
// against something that cannot be assigned to.
//
// Measured 2026-09-22 under `env -i PATH=/usr/bin:/bin`: bash 5.3.20 answers
// `$(( ++7 ))` with **7** and refuses `$(( 7++ ))` with `operand expected`
// blamed on `+ ` — neither of which is an lvalue complaint, because it never
// reads an operator there. ksh93u+ and zsh 5.9.2 take the operator and then
// refuse the target, with `assignment requires lvalue` and `bad math
// expression: lvalue required` for `7++`, `++7` and `7--` alike.
//
// So it is a grammar question, and turning it on turns
// interp.Diagnostics.ArithIncrementNeedsAPlace off (#2420).
func TestIncrementNeedsAPlaceWhereTheDialectSaysSo(t *testing.T) {
	t.Parallel()
	needs := syntax.Core()
	needs.ArithIncDecNeedsAPlace = true

	t.Run("a prefix sign pair against a literal is two signs", func(t *testing.T) {
		if err := arithErr(`++7`, needs); err != nil {
			t.Fatalf("read %q: %v, want two signs", "++7", err)
		}
		x, ok := arithOf(t, `echo $(( ++7 ))`, needs).(*syntax.ArithUnary)
		if !ok || x.Op != "+" {
			t.Fatalf("read %q: %#v, want one `+`", "++7", x)
		}
		if inner, ok := x.X.(*syntax.ArithUnary); !ok || inner.Op != "+" {
			t.Errorf("inner %#v, want a second `+`", x.X)
		}
	})

	t.Run("and against a name it is still the operator", func(t *testing.T) {
		x, ok := arithOf(t, `echo $(( ++i ))`, needs).(*syntax.ArithUnary)
		if !ok || x.Op != "++" {
			t.Fatalf("read %q: %#v, want one `++`", "++i", x)
		}
		el, ok := arithOf(t, `echo $(( ++a[0] ))`, needs).(*syntax.ArithUnary)
		if !ok || el.Op != "++" {
			t.Fatalf("read %q: %#v, want one `++`", "++a[0]", el)
		}
	})

	t.Run("a postfix pair against a literal is a sign with nothing behind it", func(t *testing.T) {
		err := arithErr(`7++ `, needs)
		var se *syntax.Error
		if !errors.As(err, &se) {
			t.Fatalf("read %q: %v, want a syntax.Error", "7++ ", err)
		}
		if se.Kind != syntax.ErrArithOperandEnd {
			t.Errorf("kind %v, want %v", se.Kind, syntax.ErrArithOperandEnd)
		}
		// The *second* sign, which is what bash names.
		if se.Token != "+ " {
			t.Errorf("token %q, want %q", se.Token, "+ ")
		}
	})

	t.Run("and against a name it is still the operator", func(t *testing.T) {
		x, ok := arithOf(t, `echo $(( i++ ))`, needs).(*syntax.ArithUnary)
		if !ok || x.Op != "++" || !x.Postfix {
			t.Fatalf("read %q: %#v, want a postfix `++`", "i++", x)
		}
	})

	t.Run("without the flag the operator is taken against anything", func(t *testing.T) {
		d := syntax.Core()
		if err := arithErr(`7++`, d); err != nil {
			t.Fatalf("read %q: %v, want the operator taken", "7++", err)
		}
		x, ok := arithOf(t, `echo $(( 7++ ))`, d).(*syntax.ArithUnary)
		if !ok || x.Op != "++" {
			t.Fatalf("read %q: %#v, want one `++`", "7++", x)
		}
	})
}

// An assignment operator standing after something that is not a place is its
// own refusal, and every shell in the panel has a sentence for it.
//
// Measured 2026-09-22: bash 5.3.20 `attempted assignment to non-variable`
// with `=4 ` as the token, ksh93u+ `assignment requires lvalue`, zsh 5.9.2
// `bad math expression: lvalue required`. It used to be text left over that
// could be no operator, which is the sentence those shells keep for `1 @`.
func TestAnAssignmentToANonPlaceIsItsOwnRefusal(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		src   string
		token string
	}{
		{`7=4 `, "=4 "},
		{`7 = 4 `, "= 4 "},
		{`(1)=2 `, "=2 "},
		{`1+2=3 `, "=3 "},
		{`x=1, 2=3 `, "=3 "},
		{`7+=4 `, "+=4 "},
	} {
		t.Run(tc.src, func(t *testing.T) {
			err := arithErr(tc.src, syntax.Core())
			var se *syntax.Error
			if !errors.As(err, &se) {
				t.Fatalf("read %q: %v, want a syntax.Error", tc.src, err)
			}
			if se.Kind != syntax.ErrArithAssignToNonPlace {
				t.Errorf("kind %v, want %v", se.Kind, syntax.ErrArithAssignToNonPlace)
			}
			if se.Token != tc.token {
				t.Errorf("token %q, want %q", se.Token, tc.token)
			}
		})
	}
	// The neighbors, which must not be caught by it: an assignment that does
	// have a place, and an equality that only looks like one.
	for _, src := range []string{`x=4`, `a[0]=4`, `x+=4`, `7==4`, `1 ? x=2 : 3`} {
		if err := arithErr(src, syntax.Core()); err != nil {
			t.Errorf("read %q: %v, want it read", src, err)
		}
	}
}

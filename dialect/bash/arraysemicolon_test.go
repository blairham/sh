// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

// Nothing but a word and a newline stands between the elements of an array
// literal here, and a refusal **names the token it found**.
//
// The wording is the half a person reads, and it used to point at a character
// that is written right there — `expected ) to close an array assignment` for
// `a=( x; )`, whose `)` is the third token along. Measured 2026-09-12 on bash
// 5.3.15, 3.2.57 and bash-as-`sh`, `-n` over a script file under `env -i`:
//
//	$ bash s.sh         # a=( x; )
//	s.sh: line 1: syntax error near unexpected token `;'
//	s.sh: line 1: `a=( x; )'
//
// ksh93 takes a single `;` ending the element list and zsh takes one wherever
// a newline stands, which is what makes this a dialect's answer rather than
// the grammar's (#1162).
func TestAnArrayLiteralRefusalNamesTheTokenHere(t *testing.T) {
	if got := bash.Dialect().SemicolonInAnArrayLiteral; got != syntax.NoSemicolonInAnArrayLiteral {
		t.Errorf("SemicolonInAnArrayLiteral = %v, want NoSemicolonInAnArrayLiteral", got)
	}
	for _, tc := range []struct{ src, token string }{
		{`a=( x; )`, ";"},
		{`a=( ; )`, ";"},
		{`a=( x; y )`, ";"},
		{`a=( x y; )`, ";"},
		{`a=( x;; y )`, ";;"},
		{`a=( x & )`, "&"},
		{`a=( x && y )`, "&&"},
	} {
		_, err := syntax.Parse(tc.src, bash.Dialect())
		var se *syntax.Error
		if !errors.As(err, &se) {
			t.Errorf("%q: err = %v, want a syntax error", tc.src, err)
			continue
		}
		if se.Token != tc.token {
			t.Errorf("%q: named %q, want %q", tc.src, se.Token, tc.token)
		}
	}
}

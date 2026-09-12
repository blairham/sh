// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/syntax"
)

func arraySemicolonDialect(v syntax.ArraySemicolon) syntax.Dialect {
	d := syntax.Core()
	d.SemicolonInAnArrayLiteral = v
	return d
}

// The three values, over the shapes that separate them. A `;` between the
// parentheses of an array literal is refused, ends the element list, or stands
// wherever a newline stands — three parses of the same characters, which is
// why this is a grammar flag rather than a semantics axis.
func TestHowFarASemicolonReachesIntoAnArrayLiteral(t *testing.T) {
	for _, tc := range []struct {
		src   string
		elems map[syntax.ArraySemicolon]int
	}{
		{`a=( x; )`, map[syntax.ArraySemicolon]int{
			syntax.NoSemicolonInAnArrayLiteral:                 -1,
			syntax.OneSemicolonEndsTheArrayElements:            1,
			syntax.SemicolonSeparatesArrayElementsLikeANewline: 1,
		}},
		{`a=( x y; )`, map[syntax.ArraySemicolon]int{
			syntax.NoSemicolonInAnArrayLiteral:                 -1,
			syntax.OneSemicolonEndsTheArrayElements:            2,
			syntax.SemicolonSeparatesArrayElementsLikeANewline: 2,
		}},
		// The terminator stands between nothing, so this is where the two
		// shells that take the row above part company.
		{`a=( x; y )`, map[syntax.ArraySemicolon]int{
			syntax.NoSemicolonInAnArrayLiteral:                 -1,
			syntax.OneSemicolonEndsTheArrayElements:            -1,
			syntax.SemicolonSeparatesArrayElementsLikeANewline: 2,
		}},
		// A terminator needs an element in front of it; a separator does not,
		// and leaves the array empty exactly as a newline would.
		{`a=( ; )`, map[syntax.ArraySemicolon]int{
			syntax.NoSemicolonInAnArrayLiteral:                 -1,
			syntax.OneSemicolonEndsTheArrayElements:            -1,
			syntax.SemicolonSeparatesArrayElementsLikeANewline: 0,
		}},
		// One of them, against as many as are written.
		{`a=( x; ; )`, map[syntax.ArraySemicolon]int{
			syntax.NoSemicolonInAnArrayLiteral:                 -1,
			syntax.OneSemicolonEndsTheArrayElements:            -1,
			syntax.SemicolonSeparatesArrayElementsLikeANewline: 1,
		}},
		// A newline is a separator under every value, which is what says the
		// flag is about the `;` and not about the element list.
		{"a=( x\ny )", map[syntax.ArraySemicolon]int{
			syntax.NoSemicolonInAnArrayLiteral:                 2,
			syntax.OneSemicolonEndsTheArrayElements:            2,
			syntax.SemicolonSeparatesArrayElementsLikeANewline: 2,
		}},
		// And the controls: `;;` stays its own token under every value, and
		// no other control operator ever stands between two elements.
		{`a=( x;; y )`, map[syntax.ArraySemicolon]int{
			syntax.NoSemicolonInAnArrayLiteral:                 -1,
			syntax.OneSemicolonEndsTheArrayElements:            -1,
			syntax.SemicolonSeparatesArrayElementsLikeANewline: -1,
		}},
		{`a=( x & )`, map[syntax.ArraySemicolon]int{
			syntax.NoSemicolonInAnArrayLiteral:                 -1,
			syntax.OneSemicolonEndsTheArrayElements:            -1,
			syntax.SemicolonSeparatesArrayElementsLikeANewline: -1,
		}},
		{`a=( x && y )`, map[syntax.ArraySemicolon]int{
			syntax.NoSemicolonInAnArrayLiteral:                 -1,
			syntax.OneSemicolonEndsTheArrayElements:            -1,
			syntax.SemicolonSeparatesArrayElementsLikeANewline: -1,
		}},
	} {
		for v, want := range tc.elems {
			f, err := syntax.Parse(tc.src, arraySemicolonDialect(v))
			if want < 0 {
				if err == nil {
					t.Errorf("%q under %v: parsed, want a refusal", tc.src, v)
				}
				continue
			}
			if err != nil {
				t.Errorf("%q under %v: %v", tc.src, v, err)
				continue
			}
			if got := arrayElemCount(t, f); got != want {
				t.Errorf("%q under %v: %d elements, want %d", tc.src, v, got, want)
			}
		}
	}
}

// A refusal names the token it found rather than asking for the `)` that is
// written right there. The dialect words it; the grammar decides what is named.
func TestAnArrayLiteralRefusalNamesTheTokenItFound(t *testing.T) {
	for _, tc := range []struct {
		src   string
		value syntax.ArraySemicolon
		token string
	}{
		{`a=( x; )`, syntax.NoSemicolonInAnArrayLiteral, ";"},
		{`a=( x;; y )`, syntax.NoSemicolonInAnArrayLiteral, ";;"},
		{`a=( x & )`, syntax.NoSemicolonInAnArrayLiteral, "&"},
		// Past a terminator only the closer may stand, so the word after one
		// is what gets named.
		{`a=( x; y )`, syntax.OneSemicolonEndsTheArrayElements, "y"},
		{`a=( x; ; )`, syntax.OneSemicolonEndsTheArrayElements, ";"},
	} {
		_, err := syntax.Parse(tc.src, arraySemicolonDialect(tc.value))
		var se *syntax.Error
		if !errors.As(err, &se) {
			t.Errorf("%q under %v: err = %v, want a syntax error", tc.src, tc.value, err)
			continue
		}
		if se.Token != tc.token {
			t.Errorf("%q under %v: named %q, want %q", tc.src, tc.value, se.Token, tc.token)
		}
	}
}

// arrayElemCount reads the element count off the one assignment in f.
func arrayElemCount(t *testing.T, f *syntax.File) int {
	t.Helper()
	if len(f.Stmts) != 1 {
		t.Fatalf("want one statement, got %d", len(f.Stmts))
	}
	pipe, ok := f.Stmts[0].Expr.(*syntax.Pipeline)
	if !ok || len(pipe.Cmds) != 1 {
		t.Fatalf("want one pipeline of one command, got %T", f.Stmts[0].Expr)
	}
	cmd, ok := pipe.Cmds[0].(*syntax.SimpleCmd)
	if !ok || len(cmd.Assigns) != 1 {
		t.Fatalf("want one simple command holding one assignment, got %T", pipe.Cmds[0])
	}
	if !cmd.Assigns[0].IsArray {
		t.Fatal("want an array assignment")
	}
	return len(cmd.Assigns[0].Elems)
}

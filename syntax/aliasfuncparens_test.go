// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// The `()` a function definition announces itself with, when the alias
// mechanism is what is holding it.
//
// An alias expansion puts its body's tokens in front of the parser rather than
// splicing its text into the input, so the lookahead has to be asked of the
// parser and not of the lexer: after `alias fn='f() {'` the parenthesis pair is
// in the pending tokens while the lexer stands past the alias word, on input
// that has nothing to do with the definition. Asked of the lexer there, the
// definition was refused — at status 2, where dash, bash 5.3, that binary as
// `sh`, bash 3.2, ksh93 and zsh all define the function and answer 0.
//
// The pair can be split three ways across the seam, and each one is a branch:
// both parentheses in the body, only the `(`, or neither because the body ends
// at the name.
func TestAnAliasBodyMayCarryTheParensOfAFunctionDefinition(t *testing.T) {
	for _, c := range []struct {
		name  string
		alias syntax.Aliases
		src   string
	}{
		{
			// Neither parenthesis is the alias's: the body ends at the name,
			// nothing is pending, and this is the path that always worked.
			// It is the control that says the other two are about the seam.
			"the body ends at the name",
			table("fn", "f"), "fn() { echo hi; }",
		},
		{
			// Only the `(` is the alias's, so the `)` is still the lexer's.
			"the body ends at the left parenthesis",
			table("fn", "f("), "fn ) { echo hi; }",
		},
		{
			"the body carries both parentheses",
			table("fn", "f()"), "fn { echo hi; }",
		},
		{
			// The body carries the brace as well, which is the spelling
			// that reads like a macro: the alias opens the definition and
			// the input closes it several lines later.
			"the body carries the parentheses and the brace",
			table("fn", "f() {"), "fn\n echo hi\n}",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := parsed(t, c.alias, c.src)
			if strings.HasPrefix(got, "error: ") {
				t.Fatalf("%q came to %q, want a function definition", c.src, got)
			}
			if !strings.Contains(got, "f()") || !strings.Contains(got, "echo hi") {
				t.Errorf("%q came to %q, want `f()` holding `echo hi`", c.src, got)
			}
		})
	}
}

// The lookahead must not become a yes for tokens that are merely pending. A
// body whose next token is not `(` is an ordinary command however it got
// there, and a `(` with no `)` after it is not a parameter list.
func TestPendingTokensAreNotAFunctionDefinitionOnTheirOwn(t *testing.T) {
	for _, c := range []struct {
		name  string
		alias syntax.Aliases
		src   string
		want  string
	}{
		{
			"a body of two ordinary words",
			table("a", "echo hi"), "a", "echo hi",
		},
		{
			// `f (x)` is not a definition in any of them: the parentheses
			// have to be empty and adjacent, and the parser may not stop
			// asking that because the tokens arrived from an alias.
			"a left parenthesis the body does not close",
			table("fn", "f("), "fn x ) ", "",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := parsed(t, c.alias, c.src)
			if strings.Contains(got, "f()") {
				t.Errorf("%q came to %q, want no function definition", c.src, got)
			}
			if c.want != "" && got != c.want {
				t.Errorf("%q came to %q, want %q", c.src, got, c.want)
			}
		})
	}
}

// The two readings that recognize the `(` from the inside — where it is
// already the current token and what is asked is the `)` after it. Both are
// zsh's, and zsh takes both through an alias: `alias af='()'` with `af { echo
// hi; }` prints `hi`, and `alias ab='a b ()'` with `ab { echo "[$0]"; }`
// defines both names. Measured 2026-09-13.
//
// They are the other side of the same seam, and they are here because a fix
// that folded only the outside half would leave a second helper asking the
// lexer the same wrong question.
func TestTheParenAlreadyReadMayBeClosedByAPendingToken(t *testing.T) {
	d := syntax.Core()
	d.AnonymousFunction = true
	d.FunctionMultipleNames = true

	t.Run("an anonymous function's empty parameter list", func(t *testing.T) {
		got := parsedIn(t, d, table("af", "()"), "af { echo hi; }")
		if strings.HasPrefix(got, "error: ") || !strings.Contains(got, "echo hi") {
			t.Errorf("came to %q, want an anonymous function holding `echo hi`", got)
		}
	})

	t.Run("two names sharing one body", func(t *testing.T) {
		// The printer writes a multiple-name definition the way it is
		// written, with one parenthesis pair for the whole list.
		got := parsedIn(t, d, table("ab", "a b ()"), "ab { echo hi; }")
		if want := "a b() { echo hi; }"; got != want {
			t.Errorf("came to %q, want %q", got, want)
		}
	})

	t.Run("a subshell is still a subshell", func(t *testing.T) {
		// The control: `(` with something other than `)` after it is a
		// subshell wherever the tokens came from, so neither reading may
		// answer yes on the strength of the tokens being pending.
		got := parsedIn(t, d, table("s", "( echo hi )"), "s")
		if strings.HasPrefix(got, "error: ") || strings.Contains(got, "()") {
			t.Errorf("came to %q, want a subshell", got)
		}
	})
}

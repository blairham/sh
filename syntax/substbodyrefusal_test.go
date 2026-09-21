// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"errors"
	"testing"
)

// Where a `$( … )` never closes, the refusal its contents raised when they
// were read as a program travels on the error, because one dialect writes it
// in front of the complaint about the substitution (#3961).
//
// Asked of the grammar rather than of a shell: the read that finds it is
// [Lexer.parseToClose], which every dialect makes, and what varies is only
// whether a dialect writes the result. See
// [interp.Diagnostics.UnterminatedSubstitutionWritesItsBodysRefusal] for the
// measurement.
func TestAnUnterminatedSubstitutionCarriesItsBodysRefusal(t *testing.T) {
	t.Parallel()
	d := Core()
	d.AnonymousFunction = true
	for _, c := range []struct {
		name, src, body string
	}{
		{"a compound left open", "v=$(echo hi; for\n", "echo hi; for\n"},
		{"a brace group left open", "v=$(echo hi; {\n", "echo hi; {\n"},
		{"a condition left open", "v=$(echo hi; if true\n", "echo hi; if true\n"},
		{"a pattern's parenthesis, and then the end", "v=$(echo hi; case x in y)\n", "echo hi; case x in y)\n"},
		{"a nameless function with no body", "v=$(echo hi; ()\n", "echo hi; ()\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(c.src, d)
			var se *Error
			if !errors.As(err, &se) {
				t.Fatalf("%q: refusal = %v, want a syntax error", c.src, err)
			}
			if se.Kind != ErrUnmatched {
				t.Fatalf("%q: kind = %v, want the substitution left unclosed", c.src, se.Kind)
			}
			if se.BodyRefusal == nil {
				t.Fatalf("%q: carried no refusal from the body", c.src)
			}
			// The body read as a body of its own, which is what the carried
			// refusal has to be: the same kind, at the same line, naming the
			// same token. Reading it as a *program* instead answers the
			// dangling-operator row below differently, which is why the
			// comparison is against a parser told what the text is.
			own := NewParser(c.body, d)
			own.InsideASubstitution()
			own.Parse()
			var want *Error
			if !errors.As(own.Err(), &want) {
				t.Fatalf("%q read on its own: refusal = %v, want a syntax error", c.body, own.Err())
			}
			got := se.BodyRefusal
			if got.Kind != want.Kind || got.Pos.Line != want.Pos.Line || got.Token != want.Token {
				t.Errorf("%q carried kind %v at line %d naming %q; the body on its own is kind %v at line %d naming %q",
					c.src, got.Kind, got.Pos.Line, got.Token, want.Kind, want.Pos.Line, want.Token)
			}
		})
	}
	// **The discriminator**: a body that parses has nothing to say, so the
	// error carries nothing and the one dialect that writes this writes the
	// quote alone.
	for _, src := range []string{"v=$(echo hi\n", "v=$(\n"} {
		_, err := Parse(src, d)
		var se *Error
		if !errors.As(err, &se) {
			t.Fatalf("%q: refusal = %v, want a syntax error", src, err)
		}
		if se.BodyRefusal != nil {
			t.Errorf("%q: carried %v, want nothing", src, se.BodyRefusal)
		}
	}
	// And the one carried refusal that brings a numbering of its own with
	// it, which is what places both messages at line 1 for `v=$(echo hi; ()`.
	_, err := Parse("v=$(echo hi; ()\n", d)
	var se *Error
	if !errors.As(err, &se) || se.BodyRefusal == nil {
		t.Fatalf("refusal = %v, want one carrying a body's", err)
	}
	if b := se.BodyRefusal; !b.FuncBody || b.FuncBodyLines != 1 {
		t.Errorf("the body's refusal is FuncBody=%v at %d lines from its parens, want true at 1",
			b.FuncBody, b.FuncBodyLines)
	}
}

// A dangling `&&` at the end of a body is the control on *which* reader that
// refusal comes from: the contents of a substitution end at the construct's
// closer, so a dialect that takes an open-ended `&&` there has nothing to
// complain about — and reading the rest of the file as a program of its own
// would refuse it and carry a message the reference never writes.
//
// Measured 2026-09-21 on zsh 5.9.2: `v=$(echo hi; echo x &&` with no closer
// writes the quote alone, where `v=$(echo hi; for` writes the body's refusal
// first. See [Parser.InsideASubstitution].
func TestAnOpenEndedOperatorAtTheEndOfABodyCarriesNoRefusal(t *testing.T) {
	t.Parallel()
	d := Core()
	d.OpenEndedAndOr = true
	for _, src := range []string{"v=$(echo hi; echo x &&\n", "v=$(echo hi; echo x ||\n"} {
		_, err := Parse(src, d)
		var se *Error
		if !errors.As(err, &se) {
			t.Fatalf("%q: refusal = %v, want a syntax error", src, err)
		}
		if se.BodyRefusal != nil {
			t.Errorf("%q: carried %v, want nothing — the body ends at the closer", src, se.BodyRefusal)
		}
	}
	// And the control for the control: the same dialect still carries the
	// refusal of a body that really has one.
	_, err := Parse("v=$(echo hi; for\n", d)
	var se *Error
	if !errors.As(err, &se) || se.BodyRefusal == nil {
		t.Errorf("a body that ran out carried nothing: %v", err)
	}
}

// Where a body's `)` was spent by the grammar, the counting loop that is the
// older answer for a substitution's extent must not spend it again — the
// substitution has no closer at all (#3961).
//
// Not a dialect question: bash 5.3.20, dash 0.5.12 and zsh 5.9.2 all take
// `v=$(echo hi; case x in y)` as a substitution that never closes, measured
// 2026-09-21. See [Lexer.lastBodyStop].
func TestAParenthesisTheGrammarSpentIsNoCloser(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		"v=$(echo hi; case x in y)\n",
		"v=$(echo hi; case x in y)\n: post\n",
		"v=$(echo hi; case a in b) echo yes;;\n",
	} {
		_, err := Parse(src, Core())
		var se *Error
		if !errors.As(err, &se) {
			t.Fatalf("%q: refusal = %v, want a syntax error", src, err)
		}
		if se.Kind != ErrUnmatched {
			t.Errorf("%q: kind = %v, want the substitution left unclosed", src, se.Kind)
		}
	}
	// **The control**, and the half the bound exists to keep: a token the
	// grammar refused rather than consumed leaves the parenthesis behind it
	// for the counting loop, which is what closes `v=$(echo hi; for)` and
	// every other body in the sweep.
	for _, src := range []string{
		"v=$(echo hi; for)\n",
		"v=$(echo hi; {)\n",
		"v=$(echo hi; case a in b) echo yes;; esac)\n",
	} {
		if _, err := Parse(src, Core()); err != nil {
			var se *Error
			if errors.As(err, &se) && se.Kind == ErrUnmatched {
				t.Errorf("%q: the substitution was left unclosed; its `)` is there to be found", src)
			}
		}
	}
}

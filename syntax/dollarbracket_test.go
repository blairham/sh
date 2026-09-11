// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// dollarBracket is the core plus the `$[expr]` spelling. The flag is named
// here and the shell that sets it is not.
func dollarBracket(on bool) syntax.Dialect {
	d := syntax.Core()
	d.DollarBracketArith = on
	return d
}

// The whole of the flag: on, `$[1+1]` is one arithmetic span holding the
// expression; off, the `$` is literal text and the brackets are a pattern,
// which is a different construct rather than a refusal.
//
// Checked on the spans rather than on a value, because the word boundary and
// the *kind* of the span are the entire difference — off, the text reaches
// pathname expansion, which is why the failure people see is `no matches
// found` and points at globbing (#900).
func TestADollarBracketIsAnArithmeticSpan(t *testing.T) {
	spans := spansOf(t, "echo $[1+1]", dollarBracket(true))
	if len(spans) != 1 {
		t.Fatalf("on: got %d spans, want 1", len(spans))
	}
	if spans[0].Kind != syntax.ArithSubst {
		t.Errorf("on: kind %v, want ArithSubst", spans[0].Kind)
	}
	if spans[0].Value != "1+1" {
		t.Errorf("on: value %q, want %q", spans[0].Value, "1+1")
	}
	if !spans[0].Bracketed {
		t.Error("on: the span does not record the spelling it was read in")
	}
	if spans[0].Arith == nil {
		t.Error("on: the expression was never parsed")
	}

	off := spansOf(t, "echo $[1+1]", dollarBracket(false))
	for _, s := range off {
		if s.Kind == syntax.ArithSubst {
			t.Fatalf("off: %q was read as arithmetic anyway", "$[1+1]")
		}
	}
	if got := joinSpanValues(off); got != "$[1+1]" {
		t.Errorf("off: the word is %q, want the text as written", got)
	}
}

// The `]` that ends it is found the way the other spelling finds its `))`:
// past nesting and past quotes. A scan taking the first one gets every shape
// here wrong, and the first is not exotic — a subscript is arithmetic too.
func TestTheClosingBracketIsFoundPastNesting(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a subscript inside", "echo $[a[1]+1]", "a[1]+1"},
		{"the spelling inside itself", "echo $[$[2+2]*2]", "$[2+2]*2"},
		{"parentheses inside", "echo $[ (1+2)*3 ]", " (1+2)*3 "},
		{"nothing at all", "echo $[]", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spans := spansOf(t, tc.src, dollarBracket(true))
			if len(spans) != 1 || spans[0].Kind != syntax.ArithSubst {
				t.Fatalf("got %d spans, want one arithmetic one", len(spans))
			}
			if spans[0].Value != tc.want {
				t.Errorf("value %q, want %q", spans[0].Value, tc.want)
			}
		})
	}
}

// A `]` inside quotes or behind a backslash does not end it either, and this
// is asked of the *lexer* rather than of a parse: every one of these is an
// arithmetic error in bash and in zsh, so a whole parse could only report
// that, and where the scan stopped is the question. bash's own diagnostic is
// what says it scanned past them — `$[']'+1]` earns `']'+1: arithmetic syntax
// error`, which is the whole text between the brackets.
func TestAQuotedBracketDoesNotEndIt(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"single quotes", "echo $[']'+1]", "']'+1"},
		{"double quotes", `echo $["]"+1]`, `"]"+1`},
		{"a backslash", `echo $[\]+1]`, `\]+1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := syntax.NewLexer(tc.src, dollarBracket(true))
			l.Next() // echo
			tok := l.Next()
			var got string
			var found bool
			for _, s := range tok.Spans {
				if s.Kind == syntax.ArithSubst {
					got, found = s.Value, true
				}
			}
			if !found {
				t.Fatalf("no arithmetic span in %q", tc.src)
			}
			if got != tc.want {
				t.Errorf("value %q, want %q", got, tc.want)
			}
		})
	}
}

// It is read wherever a substitution is read and not only in a word: inside
// double quotes, and in an unquoted here-document body. A lexer that added it
// to the word scanner alone would pass every case above and fail these.
func TestADollarBracketIsReadWhereverASubstitutionIs(t *testing.T) {
	d := dollarBracket(true)
	spans := spansOf(t, `echo "a$[1+1]b"`, d)
	var found bool
	for _, s := range spans {
		if s.Kind == syntax.ArithSubst {
			found = true
			if s.Quoting != syntax.DoubleQuoted {
				t.Errorf("in quotes: quoting %v, want DoubleQuoted", s.Quoting)
			}
			if s.Value != "1+1" {
				t.Errorf("in quotes: value %q, want %q", s.Value, "1+1")
			}
		}
	}
	if !found {
		t.Error("in quotes: no arithmetic span")
	}

	body, err := syntax.HeredocSpans("v=$[6*7]\n", d)
	if err != nil {
		t.Fatalf("in a here-document body: %v", err)
	}
	found = false
	for _, s := range body {
		if s.Kind == syntax.ArithSubst && s.Value == "6*7" {
			found = true
		}
	}
	if !found {
		t.Errorf("in a here-document body: got %d spans and no arithmetic one", len(body))
	}
	// And the flag reaches the body rather than being the word scanner's
	// alone, so a dialect without it leaves the text there too.
	off, err := syntax.HeredocSpans("v=$[6*7]\n", dollarBracket(false))
	if err != nil {
		t.Fatalf("in a here-document body with the flag off: %v", err)
	}
	for _, s := range off {
		if s.Kind == syntax.ArithSubst {
			t.Error("in a here-document body: read as arithmetic with the flag off")
		}
	}
}

// Written back in the spelling it was read in. The two are one node to
// everything that evaluates them, so a printer that normalized would be
// editing a script rather than printing it — and printing is the one thing
// that has to leave a program alone.
func TestTheSpellingSurvivesPrinting(t *testing.T) {
	for _, src := range []string{
		"echo $[1+1]",
		"echo $((1+1))",
		`echo "$[a[1]+2]"`,
		"echo $[ 1 + 1 ]",
		"x=$[2*3]",
		"echo $[$[2+2]*2]",
	} {
		f, err := syntax.Parse(src, dollarBracket(true))
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		if out := syntax.Print(f); out != src {
			t.Errorf("printed %q, want %q", out, src)
		}
	}
}

// An unfinished one is unfinished rather than literal, which is how a prompt
// knows to keep reading — the same answer the other spelling gives.
func TestAnUnterminatedDollarBracketIsIncomplete(t *testing.T) {
	_, err := syntax.Parse("echo $[1+1", dollarBracket(true))
	if err == nil {
		t.Fatal("no error for an unterminated $[")
	}
	if !strings.Contains(err.Error(), "unterminated arithmetic substitution") {
		t.Errorf("error %q, want it to name the construct", err)
	}
	// With the flag off there is nothing unterminated: the text is a word.
	if _, err := syntax.Parse("echo $[1+1", dollarBracket(false)); err != nil {
		t.Errorf("off: %v, want the text to parse as an ordinary word", err)
	}
}

// joinSpanValues is the text of a word's spans, for a case whose point is
// that no span is a substitution.
func joinSpanValues(spans []syntax.Span) string {
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(s.Value)
	}
	return b.String()
}

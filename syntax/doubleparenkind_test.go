// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// doubleParen is the core with the fallback set either way. The flag is named
// here and the shells that set it are not.
func doubleParen(on bool) syntax.Dialect {
	d := syntax.Core()
	d.ArithSubstFallsBackToCommandSubst = on
	return d
}

// The whole of the flag, and the reason it is worth having a flag at all: the
// same three bytes open two different constructs, and which one is decided
// before the parser sees a token.
//
// Checked on the span's kind rather than on a value, because the kind *is*
// the difference. Read as arithmetic, the text is an expression and the
// failure is an arithmetic one; read as a command substitution, the text is a
// program and the leading `(` is a subshell.
func TestADoubleParenMayOpenACommandSubstitution(t *testing.T) {
	for _, tc := range []struct {
		name  string
		src   string
		kind  syntax.SpanKind
		value string
	}{
		// The closing parentheses touch, so this is arithmetic and the
		// nesting inside it is part of the expression.
		{"an expression", "echo $((1+2))", syntax.ArithSubst, "1+2"},
		{"a parenthesised expression", "echo $(( (1+2) ))", syntax.ArithSubst, " (1+2) "},
		{"an empty expression", "echo $(())", syntax.ArithSubst, ""},

		// They do not touch, so this is `$(` holding a subshell, and the
		// body keeps the `(` that opens it.
		{"a subshell", "echo $((echo hi) )", syntax.CommandSubst, "(echo hi) "},
		{"a subshell after a blank", "echo $(( echo hi ) )", syntax.CommandSubst, "( echo hi ) "},
		{"two subshells", "echo $(((echo a); (echo b)) )", syntax.CommandSubst, "((echo a); (echo b)) "},

		// The count reaching zero is what decides it, not whether the text
		// would have made an expression. `(1) + (2)` is good arithmetic and
		// this is still a command substitution, because the first `)`
		// closes the count and a `+` follows it.
		{"an expression the count ends early", "echo $(( 1 ) + (2 ))", syntax.CommandSubst, "( 1 ) + (2 )"},
		{"a nested group that closes first", "echo $(( (1+2)) )", syntax.CommandSubst, "( (1+2)) "},

		// Quoting counts, so a `)` inside a string closes nothing and the
		// construct is arithmetic after all.
		{"a closer inside double quotes", `echo $(( "0)" ))`, syntax.ArithSubst, ` "0)" `},
		{"a closer inside single quotes", `echo $(( '0)' ))`, syntax.ArithSubst, ` '0)' `},
		{"an escaped closer", `echo $(( \) ))`, syntax.ArithSubst, ` \) `},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spans := spansOf(t, tc.src, doubleParen(true))
			if len(spans) != 1 {
				t.Fatalf("got %d spans, want 1", len(spans))
			}
			if spans[0].Kind != tc.kind {
				t.Errorf("kind %v, want %v", spans[0].Kind, tc.kind)
			}
			if spans[0].Value != tc.value {
				t.Errorf("value %q, want %q", spans[0].Value, tc.value)
			}
		})
	}
}

// Off, there is one reading and it is arithmetic — which is what the two
// shells without the fallback say by refusing the text for a missing `))`
// rather than by running anything.
//
// Checked on the kind rather than on a refusal, because an expression is not
// read until it is expanded: off, this parses, and what it parsed to is an
// arithmetic span holding text no expression parser will take.
func TestWithoutTheFallbackADoubleParenIsAlwaysArithmetic(t *testing.T) {
	for _, src := range []string{"echo $((echo hi) )", "echo $(( 1 ) + (2 ))", "echo $((1+2))"} {
		spans := spansOf(t, src, doubleParen(false))
		if len(spans) != 1 || spans[0].Kind != syntax.ArithSubst {
			t.Errorf("off: %q gave %v, want one arithmetic span", src, kindsOf(spans))
		}
	}
}

// Input that runs out inside a `$((` is left to arithmetic, and the flag does
// not change that. The scan reaches the end without the count ever returning
// to zero, so there is no `)` to ask the question of — and blaming a command
// substitution for text that closed neither construct would move the
// complaint to a construct nobody wrote.
func TestAnUnfinishedDoubleParenStaysArithmetic(t *testing.T) {
	for _, src := range []string{"echo $((1+2", "echo $((echo hi"} {
		_, err := syntax.Parse(src, doubleParen(true))
		if err == nil {
			t.Fatalf("%q: accepted, want a refusal", src)
		}
		_, offErr := syntax.Parse(src, doubleParen(false))
		if offErr == nil {
			t.Fatalf("%q: accepted with the flag off, want a refusal", src)
		}
		if err.Error() != offErr.Error() {
			t.Errorf("%q: on says %q, off says %q — the flag moved an unfinished construct's complaint",
				src, err.Error(), offErr.Error())
		}
	}
}

// A double quote is a second place the three bytes are read, and it has to
// take the same route. It is a separate site in the lexer, and a fix applied
// to one site and not the other is how a helper spreads a bug instead of
// ending it.
//
// The third site is a here-document body, whose spans are not cut at parse
// time — the body is one literal until it is expanded — so that route is
// graded by running it, in the corpus rather than here.
func TestTheFallbackAppliesInsideDoubleQuotes(t *testing.T) {
	spans := spansOf(t, `echo "$((echo hi) )"`, doubleParen(true))
	if len(spans) != 1 || spans[0].Kind != syntax.CommandSubst {
		t.Fatalf("got %v, want one command substitution", kindsOf(spans))
	}
	if spans[0].Quoting != syntax.DoubleQuoted {
		t.Errorf("quoting %v, want DoubleQuoted", spans[0].Quoting)
	}
}

func kindsOf(spans []syntax.Span) []syntax.SpanKind {
	kinds := make([]syntax.SpanKind, len(spans))
	for i, s := range spans {
		kinds[i] = s.Kind
	}
	return kinds
}

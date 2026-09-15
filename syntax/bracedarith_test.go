// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// firstSpan returns the first span of the first argument word, which is where
// every case below writes its expansion.
func firstArgSpan(t *testing.T, src string, d syntax.Dialect) (syntax.Span, bool) {
	t.Helper()
	f, err := syntax.Parse(src, d)
	if err != nil {
		return syntax.Span{}, false
	}
	for _, st := range f.Stmts {
		p, ok := st.Expr.(*syntax.Pipeline)
		if !ok {
			continue
		}
		for _, cmd := range p.Cmds {
			s, ok := cmd.(*syntax.SimpleCmd)
			if !ok {
				continue
			}
			for _, w := range s.Args[1:] {
				if len(w.Spans) > 0 {
					return w.Spans[0], true
				}
			}
		}
	}
	return syntax.Span{}, false
}

// `${((expr))}` is an arithmetic expansion where the dialect has it, and the
// two adjacent parens are the whole of the discriminator: one space makes it
// the subshell spelling, whose body is a command list.
func TestABracedArithmeticExpansionIsToldByTheAdjacentParens(t *testing.T) {
	d := syntax.Core()
	d.BracedArithmeticExpansion = true
	d.SubshellSubstitution = true
	for _, c := range []struct {
		name, src string
		kind      syntax.SpanKind
		value     string
	}{
		{"adjacent parens are arithmetic", "echo ${((1+2))}", syntax.ArithSubst, "1+2"},
		{"blanks inside do not change it", "echo ${(( 1+2 ))}", syntax.ArithSubst, " 1+2 "},
		{"a third paren is part of the expression", "echo ${(((1+2)))}", syntax.ArithSubst, "(1+2)"},
		{"a word may follow it", "echo ${((1+2))}x", syntax.ArithSubst, "1+2"},
		{"one space and it is the subshell spelling", "echo ${( (1+2) )}", syntax.CommandSubst, "( (1+2) )"},
	} {
		t.Run(c.name, func(t *testing.T) {
			sp, ok := firstArgSpan(t, c.src, d)
			if !ok {
				t.Fatalf("%q was refused; want it read", c.src)
			}
			if sp.Kind != c.kind {
				t.Fatalf("%q kind = %v, want %v", c.src, sp.Kind, c.kind)
			}
			if sp.Value != c.value {
				t.Errorf("%q value = %q, want %q", c.src, sp.Value, c.value)
			}
			if braced := sp.Kind == syntax.ArithSubst && sp.Braced; braced != (c.kind == syntax.ArithSubst) {
				t.Errorf("%q read as braced arithmetic = %v, want %v", c.src, braced, c.kind == syntax.ArithSubst)
			}
		})
	}
}

// The brace has to sit directly behind the `))` with nothing in between, and
// a construct that never closes is refused rather than read as something else.
func TestABracedArithmeticExpansionNeedsItsBraceBehindTheParens(t *testing.T) {
	d := syntax.Core()
	d.BracedArithmeticExpansion = true
	for _, src := range []string{
		"echo ${((1+2)) }",
		"echo ${((1+2))",
		"echo ${((1+2)",
	} {
		if _, err := syntax.Parse(src, d); err == nil {
			t.Errorf("%q parsed; want a refusal", src)
		}
	}
}

// Without the flag the spelling stays exactly the refusal it was: the `${`
// goes on to be read as a parameter whose name begins with a paren, which is
// not a name.
func TestWithoutTheFlagTheBracedSpellingIsRefused(t *testing.T) {
	sp, ok := firstArgSpan(t, "echo ${((1+2))}", syntax.Core())
	if !ok {
		t.Fatalf("`${((1+2))}` was refused while reading; the dialects without the construct defer it to the run")
	}
	if sp.Kind == syntax.ArithSubst || sp.Braced {
		t.Errorf("read as arithmetic without the flag: kind %v, braced %v", sp.Kind, sp.Braced)
	}
	if sp.Kind != syntax.ParamExp {
		t.Errorf("kind = %v, want a parameter expansion whose name is not one", sp.Kind)
	}
}

// The spelling that was read is the spelling that is written back — the two
// are one node and are not one syntax, so normalizing would edit a script.
func TestABracedArithmeticExpansionIsPrintedAsItWasRead(t *testing.T) {
	d := syntax.Core()
	d.BracedArithmeticExpansion = true
	const src = "echo ${((1+2))}x"
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := syntax.Print(f); got != src {
		t.Errorf("printed %q, want %q", got, src)
	}
}

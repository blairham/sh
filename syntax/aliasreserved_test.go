// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// Whether an alias may stand in for a word the grammar reserves.
//
// The standard puts such a name out of bounds and two shells take it anyway,
// so it is one field — [syntax.Dialect.AliasesExpandReservedWords] — and the
// panel behind it is on that field's own documentation. What is asserted here
// is the two halves a measurement cannot assert: that the flag reaches every
// member of the reserved set, and that the set is **this dialect's** rather
// than the union.

// reservedWordsInEveryDialect are the words no preset can be without. The
// three a preset adds — `select`, `function`, `time` — are deliberately not
// here; they are the subject of the second test below.
var reservedWordsInEveryDialect = []string{
	"if", "then", "elif", "else", "fi",
	"for", "while", "until", "do", "done",
	"case", "in", "esac", "{", "}", "!",
}

// parsedIn is parsed() for a test that has to name the dialect, since the
// dialect is what this file is about.
func parsedIn(t *testing.T, d syntax.Dialect, a syntax.Aliases, src string) string {
	t.Helper()
	p := syntax.NewParser(src, d)
	p.Aliases = a
	f := p.Parse()
	if err := p.Err(); err != nil {
		return "error: " + err.Error()
	}
	return strings.TrimSpace(syntax.Print(f))
}

// wholeGrammar is a dialect holding all three of the constructs a preset adds,
// so that a word being protected there is about the flag and not about the
// grammar missing the construct.
func wholeGrammar(expand bool) syntax.Dialect {
	d := syntax.Core()
	d.Select = true
	d.FunctionKeyword = true
	d.TimeKeyword = true
	d.AliasesExpandReservedWords = expand
	return d
}

// The flag reaches every reserved word rather than the one the suite happened
// to use. Asserted as a difference between the two dialects — whether the
// alias body is in what was parsed — because what a *declined* substitution
// comes to varies by word: a bare `for` is a syntax error, a bare `in` is a
// command nobody has, and neither of those is the fact under test.
func TestAnAliasStandsInForAReservedWordOnlyWhereTheDialectSaysSo(t *testing.T) {
	for _, w := range reservedWordsInEveryDialect {
		t.Run(w, func(t *testing.T) {
			a, src := table(w, "echo took"), w
			if w == "!" {
				// `!` is reached one position further in. At the head of a
				// pipeline the parser answers it as the negation before a
				// table is consulted at all — a gap of its own, since bash
				// and zsh both substitute there — so the position this word
				// is reachable in is the one after a value ending in a
				// blank, which is where the corpus row asks it too.
				a, src = table("sp", " ", w, "echo took"), "sp ! true"
			}

			if got := parsedIn(t, wholeGrammar(true), a, src); !strings.Contains(got, "echo took") {
				t.Errorf("with the flag on, %q came to %q, want the alias to have won", src, got)
			}
			if got := parsedIn(t, wholeGrammar(false), a, src); strings.Contains(got, "echo took") {
				t.Errorf("with the flag off, %q took the alias; the grammar's own word must survive", src)
			}
		})
	}

	// The name is still *stored* — only the substitution is declined — which
	// is what keeps `alias` and `unalias` working on it. The parser cannot
	// see the table, so the observable is that the word after a declined one
	// is not swallowed: the alias table is consulted again for it.
	d := wholeGrammar(false)
	if got := parsedIn(t, d, table("for", "echo took", "hi", "echo yes"), "hi"); got != "echo yes" {
		t.Errorf("an ordinary name after a protected one came to %q, want echo yes", got)
	}
}

// The protected set is the words *that dialect* reserves, which is what makes
// this one field rather than a table of names. Measured 2026-09-13 from a
// script file: dash has no `select`, no `function` keyword and no `time`
// keyword and takes an alias for all three; BusyBox ash has `function` alone
// and protects exactly that one; ksh93 has all three and protects all three.
func TestTheProtectedSetIsTheWordsThisGrammarReserves(t *testing.T) {
	shape := func(sel, fn, tm bool) syntax.Dialect {
		d := syntax.Core()
		d.Select, d.FunctionKeyword, d.TimeKeyword = sel, fn, tm
		d.AliasesExpandReservedWords = false
		return d
	}

	for _, c := range []struct {
		name    string
		dialect syntax.Dialect
		// took lists the words whose alias still stands in, because this
		// grammar does not reserve them.
		took []string
		// kept lists the words the grammar holds and therefore protects.
		kept []string
	}{
		{
			"none of the three, as dash has none", shape(false, false, false),
			[]string{"select", "function", "time"},
			nil,
		},
		{
			"`function` alone, as BusyBox ash has", shape(false, true, false),
			[]string{"select", "time"},
			[]string{"function"},
		},
		{
			"all three, as ksh93 has", shape(true, true, true),
			nil,
			[]string{"select", "function", "time"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, w := range c.took {
				if got := parsedIn(t, c.dialect, table(w, "echo took"), w); got != "echo took" {
					t.Errorf("%q is not a word this grammar reserves, so its alias must stand in; got %q", w, got)
				}
			}
			for _, w := range c.kept {
				if got := parsedIn(t, c.dialect, table(w, "echo took"), w); got == "echo took" {
					t.Errorf("%q is a word this grammar reserves, so its alias must not stand in", w)
				}
			}
		})
	}
}

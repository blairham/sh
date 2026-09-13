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
				// A bare `!` is a pipeline with nothing in it, which is its
				// own question — see Dialect.BareNegationReach — so the word
				// is asked here in front of a command. Both positions `!`
				// stands in are covered: this one by
				// TestThePipelineWordsAreAskedOfTheTableFirst and the one
				// after a value ending in a blank by the loop below.
				src = "! true"
			}

			if got := parsedIn(t, wholeGrammar(true), a, src); !strings.Contains(got, "echo took") {
				t.Errorf("with the flag on, %q came to %q, want the alias to have won", src, got)
			}
			if got := parsedIn(t, wholeGrammar(false), a, src); strings.Contains(got, "echo took") {
				t.Errorf("with the flag off, %q took the alias; the grammar's own word must survive", src)
			}
		})
	}

	// The other position `!` stands in: a value ending in a blank makes the
	// word after it eligible, and the word made eligible that way is judged
	// by the same reservation. It is the control the corpus row
	// `alias/a-reserved-word-after-an-alias-ending-in-a-blank` is written
	// around, and the one position the flag reached before #2638.
	t.Run("! after a value ending in a blank", func(t *testing.T) {
		a := table("sp", " ", "!", "echo took")
		if got := parsedIn(t, wholeGrammar(true), a, "sp ! true"); !strings.Contains(got, "echo took") {
			t.Errorf("with the flag on, `sp ! true` came to %q, want the alias to have won", got)
		}
		if got := parsedIn(t, wholeGrammar(false), a, "sp ! true"); strings.Contains(got, "echo took") {
			t.Errorf("with the flag off, `sp ! true` took the alias; the negation must survive")
		}
	})

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

// # The two words a pipeline reads for itself
//
// Every other reserved word is read by parseCommand, which offers the word to
// the alias table before it dispatches on the keyword. A pipeline's leading
// `!` and the `time` in front of it are read one level out, in parsePipeline,
// before there is a command to parse — so until #2638 nothing asked the table
// there at all, and the flag that decides every other reserved word did not
// reach these two.
//
// Measured 2026-09-13 from a script file. `alias '!'='echo took'` with `!
// true` behind it prints `took true` and answers 0 in bash 5.3, bash 3.2 and
// zsh, and prints nothing and answers 1 in bash as `sh`, zsh as `sh`, dash,
// ksh93 and BusyBox ash. `alias time='echo took;'` with `time true` behind it
// splits the same way, with dash and ash on the taking side because neither
// has the keyword to protect. That is the split
// [syntax.Dialect.AliasesExpandReservedWords] already holds, so it is this
// field reaching further rather than an axis of its own.

// pipelineHeads are the places the word a pipeline begins with is read from.
// A pipeline stands in all of them, so the substitution has to reach all of
// them: the head of a line, the right of an `&&`, an `if` condition, and the
// far side of a bar.
var pipelineHeads = []struct {
	name string
	src  string
}{
	{"a line of its own", "! true"},
	{"the right of an &&", "true && ! true"},
	{"an if condition", "if ! true; then echo y; fi"},
	{"the far side of a bar", "echo x | ! cat"},
}

func TestThePipelineWordsAreAskedOfTheTableFirst(t *testing.T) {
	t.Run("!", func(t *testing.T) {
		for _, c := range pipelineHeads {
			t.Run(c.name, func(t *testing.T) {
				a := table("!", "echo took")
				if got := parsedIn(t, wholeGrammar(true), a, c.src); !strings.Contains(got, "echo took") {
					t.Errorf("with the flag on, %q came to %q, want the alias to have won", c.src, got)
				}
				if got := parsedIn(t, wholeGrammar(false), a, c.src); strings.Contains(got, "echo took") {
					t.Errorf("with the flag off, %q took the alias; the negation must survive", c.src)
				}
			})
		}
	})

	// `time` is asked only where the grammar has the keyword, which is the
	// same question reservedInDialect asks about it: a dialect without `time`
	// never reads the word here, and parseCommand expands it as an ordinary
	// command name. That is why dash and ash take this alias while ksh93 does
	// not, and it is what makes the protected set the grammar's own.
	t.Run("time", func(t *testing.T) {
		a := table("time", "echo took;")
		if got := parsedIn(t, wholeGrammar(true), a, "time true"); !strings.Contains(got, "echo took") {
			t.Errorf("with the flag on, `time true` came to %q, want the alias to have won", got)
		}
		if got := parsedIn(t, wholeGrammar(false), a, "time true"); strings.Contains(got, "echo took") {
			t.Error("with the flag off, `time true` took the alias; the keyword must survive")
		}
		d := wholeGrammar(false)
		d.TimeKeyword = false
		if got := parsedIn(t, d, a, "time true"); !strings.Contains(got, "echo took") {
			t.Errorf("a grammar without the keyword protects nothing, so `time true` should have taken the alias; got %q", got)
		}
	})
}

// An ordinary alias at the head of a pipeline must run without the dialect
// being consulted at all: the reservation is about the *name*, and a name the
// grammar does not reserve is expanded there exactly as it is anywhere a
// command word stands.
//
// Asserted as the answer being the same on both sides of the flag, which is
// what falsifies the over-reach this fix could have been written as — asking
// the flag about every word a pipeline begins with, and so letting a dialect
// that protects reserved words swallow `e hi` too.
func TestAnOrdinaryHeadAsksTheReservedWordFieldNothing(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a line of its own", "e hi", "echo hi"},
		{"behind a negation", "! e hi", "! echo hi"},
		{"the right of an &&", "true && e hi", "true && echo hi"},
		{"the far side of a bar", "true | e hi", "true | echo hi"},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, expand := range []bool{true, false} {
				if got := parsedIn(t, wholeGrammar(expand), table("e", "echo"), c.src); got != c.want {
					t.Errorf("with the flag %v, %q came to %q, want %q", expand, c.src, got, c.want)
				}
			}
		})
	}
}

// The substitution reaches the negation and nothing else `!` means. The other
// two are read by parseCond and by parseParamExp, from tokens parsePipeline
// never sees, and a table holding `!` must leave both alone — with the flag
// **on**, which is the only setting that could have reached them.
func TestTheHeadExpansionReachesNoOtherBang(t *testing.T) {
	d := wholeGrammar(true)
	d.DoubleBracket = true
	d.ParamIndirection = true
	a := table("!", "echo took")

	for _, c := range []struct{ name, src, want string }{
		{"the negation inside [[ ]]", "[[ ! -n x ]]", "[[ ! -n x ]]"},
		{"an indirect parameter expansion", "echo ${!v}", "echo ${!v}"},
		{"a quoted word is not the name", `"!" true`, `"!" true`},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := parsedIn(t, d, a, c.src); got != c.want {
				t.Errorf("%q came to %q, want %q", c.src, got, c.want)
			}
		})
	}
}

// The two rules the existing machinery carries have to survive the new door,
// because the head expansion is the same expansion and not a second copy of
// one.
//
//   - A value ending in a blank makes the word after it eligible. `alias
//     '!'='echo '` with `alias hi='echo HI'` behind it is `echo echo HI` in
//     all three shells that expand here — the second `echo` is the alias `hi`
//     expanded in turn — so the flag the splice sets must not be cleared by
//     parseCommand offering the same word a second time.
//   - A body that names itself stops. `alias '!'='echo took;!'` prints `took`
//     and then answers 1, rather than expanding forever: the `!` the body
//     ends with is inside the chain `!` opened, so it is read as the negation
//     of nothing. That is Parser.pendingChains, and this asserts the head
//     door reaches it.
func TestTheHeadExpansionKeepsTheRestOfTheAlgorithm(t *testing.T) {
	d := wholeGrammar(true)
	if got := parsedIn(t, d, table("!", "echo ", "hi", "echo HI"), "! hi"); got != "echo echo HI" {
		t.Errorf("`! hi` came to %q, want the blank to have carried the expansion on", got)
	}
	if got := parsedIn(t, d, table("!", "echo took;!"), "! true"); !strings.Contains(got, "echo took") {
		t.Errorf("`! true` came to %q, want the body to have been taken once", got)
	}

	// The word *behind* a `!` the head expansion produced is an ordinary
	// command word and has never been offered to anything, so parseCommand
	// still has to offer it. `alias '!'='! x'` is `x: command not found` at
	// 0 in all three shells that expand here — the body's `!` is the
	// negation, because it is inside the chain `!` opened — and `x` is
	// looked up like any other name.
	if got := parsedIn(t, d, table("!", "! x", "x", "echo X"), "! true"); got != "! echo X true" {
		t.Errorf("`! true` came to %q, want `! echo X true`", got)
	}
}

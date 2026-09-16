// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	. "github.com/blairham/sh/syntax"
)

// Reserves answers from the flags and never from a list of names, which is
// the whole of why it is here: an interpreter asking "is this a word of the
// grammar" is asking about the grammar it was given, and the answer moves
// with the dialect (#2918).
//
// Each row names the flag it turns on, never a shell — what a shell holds is
// that shell's package's to say.
func TestReservesFollowsTheFlagThatAddsTheConstruct(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		word string
		on   func(*Dialect)
	}{
		{"Select", "select", func(d *Dialect) { d.Select = true }},
		{"FunctionKeyword", "function", func(d *Dialect) { d.FunctionKeyword = true }},
		{"TimeKeyword", "time", func(d *Dialect) { d.TimeKeyword = true }},
		{"Coproc", "coproc", func(d *Dialect) { d.Coproc = true }},
		{"DoubleBracket opening", "[[", func(d *Dialect) { d.DoubleBracket = true }},
		{"DoubleBracket closing", "]]", func(d *Dialect) { d.DoubleBracket = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bare := POSIX()
			if bare.Reserves(tc.word) {
				t.Errorf("a dialect without the flag reserves %q", tc.word)
			}
			with := POSIX()
			tc.on(&with)
			if !with.Reserves(tc.word) {
				t.Errorf("a dialect with the flag does not reserve %q", tc.word)
			}
		})
	}
}

// The words every dialect reserves are reserved in the barest one, which is
// what makes the rows above about the constructs a flag adds rather than
// about the vocabulary as a whole.
func TestReservesTakesTheWordsEveryGrammarHas(t *testing.T) {
	t.Parallel()
	bare := POSIX()
	for _, word := range []string{
		"if", "then", "elif", "else", "fi", "for", "while", "until", "do",
		"done", "case", "in", "esac", "{", "}", "!",
	} {
		if !bare.Reserves(word) {
			t.Errorf("the barest grammar does not reserve %q", word)
		}
	}
	// And a name is a name. `local` is a builtin in most of the panel and a
	// word in none of it, which is the kind of answer a written-out list
	// drifts into.
	for _, word := range []string{"local", "echo", "typeset", "let", "[", "test", ""} {
		if bare.Reserves(word) {
			t.Errorf("%q is reserved, want it read as a name", word)
		}
	}
}

// The two ends of the conditional command are spelled as operators, so they
// are not words the lexer classes as reserved — and an alias expansion asks
// that narrower question. The check is that the two answers really differ,
// because a Reserves that had quietly become the lexer's own set would still
// pass every row above.
func TestTheClosingBracketIsReservedWithoutBeingAWord(t *testing.T) {
	t.Parallel()
	d := POSIX()
	d.DoubleBracket = true
	if !d.Reserves("]]") {
		t.Fatal("]] is not reserved in a grammar that has the construct")
	}
	// An alias may still be named after it, which is the question
	// Parser.reservedInDialect asks and this one does not.
	d.AliasesExpandUnlessTold = true
	d.ExpandAliasesInProgramText = RouteOnEveryRoute
	f, err := Parse("alias ']]'=echo\n]] hi\n", d)
	if err != nil {
		t.Fatalf("an alias named after the closing bracket was refused: %v", err)
	}
	if len(f.Stmts) != 2 {
		t.Fatalf("parsed %d statements, want 2", len(f.Stmts))
	}
}

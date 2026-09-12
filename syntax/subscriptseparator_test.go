// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// A subscript written at command position holds its separators (#2410).
//
// `m[foo bar]=qux` was cut at the blank, so the assignment was never
// recognized and `m[foo` was run as a command — while `${m[foo bar]}` on the
// read side already worked, which is what made the bug a write-only one.
//
// Measured 2026-09-12 on bash 5.3.15, bash 3.2.57, that build invoked as
// `sh`, and ksh93u+, every row read back with `typeset -p m`: once a name at
// command position is followed by `[`, the text runs to the *matching* `]`
// and everything inside is ordinary. zsh 5.9.2 refuses the same text with
// `bad pattern: m[foo`, and dash and BusyBox ash have no arrays, so the flag
// gives a reading to text three shells do not read at all.
//
// The rows are written as the *subscript* rather than as "it parsed",
// because parsing is the cheap half: a reading that admitted the line and
// then dropped or collapsed the blanks would parse every row here and store
// the wrong key.

// subscriptSeparatorGrammar is the grammar this construct needs, named by the
// construct. ArraySubscript is what makes the brackets a subscript at all;
// this flag only decides where the word ends.
func subscriptSeparatorGrammar() syntax.Dialect {
	d := syntax.Core()
	d.SubscriptSpansSeparators = true
	return d
}

func TestASubscriptAtCommandPositionHoldsItsSeparators(t *testing.T) {
	d := subscriptSeparatorGrammar()
	for _, c := range []struct {
		name, src    string
		index, value string
		appends      bool
	}{
		{
			// The issue's own shape.
			name: "a blank", src: "m[foo bar]=qux",
			index: `foo\ bar`, value: "qux",
		},
		{
			// A run is not collapsed, which is what says the subscript is
			// the text and not a join of two words with one space.
			name: "a run of blanks", src: "m[foo  bar]=qux",
			index: `foo\ \ bar`, value: "qux",
		},
		{
			name: "a tab", src: "m[foo\tbar]=qux",
			index: "foo\\\tbar", value: "qux",
		},
		{
			// The same cut showed on the append spelling, and it is the
			// same bug rather than a second one.
			name: "the append spelling", src: `m[foo bar]+=" blat"`,
			index: `foo\ bar`, value: `" blat"`, appends: true,
		},
		{
			// Not only blanks: measured, `m[a; b]=v` is the key `a; b`, so
			// every operator inside the brackets is text too.
			name: "a semicolon", src: "m[a; b]=v",
			index: `a\;\ b`, value: "v",
		},
		{
			name: "a pipe", src: "m[a|b]=v",
			index: `a\|b`, value: "v",
		},
		{
			name: "a redirection operator", src: "m[a>b]=v",
			index: `a\>b`, value: "v",
		},
		{
			name: "an and-and", src: "m[a && b]=v",
			index: `a\ \&\&\ b`, value: "v",
		},
		{
			// A `#` inside the brackets is not a comment: it is a key.
			name: "a hash", src: "m[#c]=v",
			index: "#c", value: "v",
		},
		{
			// Brackets nest, so it is the *matching* `]` that ends it and
			// not the first one.
			name: "a nested bracket", src: "m[a [b] c]=v",
			index: `a\ [b]\ c`, value: "v",
		},
		{
			// A newline is a separator like any other in there, and the
			// word spans the line.
			name: "a newline", src: "m[foo\nbar]=v",
			index: "foo'\n'bar", value: "v",
		},
		{
			// A quoted or escaped `]` closes nothing, which is what says the
			// depth is counted over unquoted literal text alone.
			name: "a quoted close bracket", src: `m['a]b' c]=v`,
			index: `'a]b'\ c`, value: "v",
		},
		{
			name: "an escaped close bracket", src: `m[a\] b]=v`,
			index: `a\]\ b`, value: "v",
		},
		{
			// And an expansion may still stand in one: the bracket a
			// substitution produces was never written.
			name: "a substitution beside the blank", src: "m[$k b]=v",
			index: `$k\ b`, value: "v",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, err := syntax.Parse(c.src, d)
			if err != nil {
				t.Fatalf("parse %q: %v", c.src, err)
			}
			cmd := onlySimple(t, f)
			if len(cmd.Assigns) != 1 || len(cmd.Args) != 0 {
				t.Fatalf("%d assignments and %d words, want 1 and 0",
					len(cmd.Assigns), len(cmd.Args))
			}
			a := cmd.Assigns[0]
			if a.Name != "m" {
				t.Errorf("name = %q, want %q", a.Name, "m")
			}
			if a.Index == nil {
				t.Fatalf("no subscript, want %q", c.index)
			}
			if got := syntax.PrintWord(a.Index); got != c.index {
				t.Errorf("subscript = %q, want %q", got, c.index)
			}
			got := ""
			if a.Value != nil {
				got = syntax.PrintWord(a.Value)
			}
			if got != c.value {
				t.Errorf("value = %q, want %q", got, c.value)
			}
			if a.Append != c.appends {
				t.Errorf("append = %v, want %v", a.Append, c.appends)
			}
		})
	}
}

// The `=` is not part of the condition, because the shells do not make it
// one: `m[foo bar]` alone is quoted back whole as `m[foo bar]: command not
// found` in all four that span separators, so the word is one word before
// anything has looked for an assignment.
func TestASubscriptedWordWithNoAssignmentIsStillOneWord(t *testing.T) {
	f, err := syntax.Parse("m[foo bar]", subscriptSeparatorGrammar())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cmd := onlySimple(t, f)
	if len(cmd.Args) != 1 {
		t.Fatalf("%d words, want 1", len(cmd.Args))
	}
	if got := syntax.PrintWord(cmd.Args[0]); got != `m[foo\ bar]` {
		t.Errorf("word = %q, want %q", got, `m[foo\ bar]`)
	}
}

// Without the flag the word is cut at the blank, which is what the three
// shells that do not have the construct make of it and what this grammar
// answers by default. The row is here so the flag cannot be deleted and
// leave the tests green.
func TestWithoutTheFlagTheSeparatorEndsTheWord(t *testing.T) {
	for _, src := range []string{
		"m[foo bar]=qux",
		"m[foo bar]+=qux",
		"m[foo\tbar]=qux",
	} {
		f, err := syntax.Parse(src, syntax.Core())
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		cmd := onlySimple(t, f)
		if len(cmd.Assigns) != 0 {
			t.Errorf("%q: %d assignments, want none without the flag",
				src, len(cmd.Assigns))
		}
		if len(cmd.Args) != 2 {
			t.Errorf("%q: %d words, want 2 without the flag", src, len(cmd.Args))
		}
	}
}

// An argument is not command position, and that is measured rather than
// assumed: `printf '<%s>' m[foo bar]=v` prints two fields in every shell that
// spans separators at the front of a command. Without this row the flag would
// read as "a subscript holds its blanks", which is a larger claim than the
// shells make.
func TestAnArgumentsSubscriptStillEndsAtTheBlank(t *testing.T) {
	f, err := syntax.Parse("printf x m[foo bar]=v", subscriptSeparatorGrammar())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cmd := onlySimple(t, f)
	if len(cmd.Args) != 4 {
		t.Fatalf("%d words, want 4", len(cmd.Args))
	}
}

// A name is required in front of the bracket, and it is required to be a
// name. Measured: `1m[foo bar]=v`, `m-n[foo bar]=v` and `[foo bar]=v` all
// still end at the blank in the four shells that take the construct.
func TestOnlyANameOpensASpanningSubscript(t *testing.T) {
	d := subscriptSeparatorGrammar()
	// A bare `[` opening the word is a word of its own in this grammar, so
	// it is asked for two words rather than for an assignment.
	for _, src := range []string{
		"1m[foo bar]=v",
		"m-n[foo bar]=v",
		"[foo bar]=v",
		`"m"[foo bar]=v`,
		"$n[foo bar]=v",
	} {
		f, err := syntax.Parse(src, d)
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		cmd := onlySimple(t, f)
		if len(cmd.Args)+len(cmd.Assigns) < 2 {
			t.Errorf("%q: %d words and %d assignments, want the blank to have cut it",
				src, len(cmd.Args), len(cmd.Assigns))
		}
	}
}

// A bracket with no matching `]` leaves the word exactly as a grammar without
// the flag reads it.
//
// The two shells that span separators diverge there — bash refuses with
// `unexpected EOF while looking for matching ']'` and ksh93 swallows the rest
// of the input into the word — so there is no common answer to implement, and
// falling back is what keeps the flag additive: no input that parsed before
// parses differently now.
func TestAnUnmatchedBracketFallsBackToTheOrdinaryReading(t *testing.T) {
	f, err := syntax.Parse("m[a b; echo done", subscriptSeparatorGrammar())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(f.Stmts) != 2 {
		t.Fatalf("%d statements, want 2", len(f.Stmts))
	}
	pipe, ok := f.Stmts[0].Expr.(*syntax.Pipeline)
	if !ok || len(pipe.Cmds) != 1 {
		t.Fatalf("first statement is %T, want a one-command pipeline", f.Stmts[0].Expr)
	}
	cmd, ok := pipe.Cmds[0].(*syntax.SimpleCmd)
	if !ok {
		t.Fatalf("first command is %T, want a simple command", pipe.Cmds[0])
	}
	if len(cmd.Args) != 2 {
		t.Errorf("%d words, want 2 — the blank should still have cut it", len(cmd.Args))
	}
}

// An assignment prefix is command position too, so a second one on the same
// line spans separators as well: measured, `a=1 m[foo bar]=v` writes the
// element in bash 5.3.15 and ksh93u+.
func TestAnAssignmentPrefixIsCommandPositionToo(t *testing.T) {
	f, err := syntax.Parse("a=1 m[foo bar]=v", subscriptSeparatorGrammar())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cmd := onlySimple(t, f)
	if len(cmd.Assigns) != 2 || len(cmd.Args) != 0 {
		t.Fatalf("%d assignments and %d words, want 2 and 0",
			len(cmd.Assigns), len(cmd.Args))
	}
	if got := syntax.PrintWord(cmd.Assigns[1].Index); got != `foo\ bar` {
		t.Errorf("subscript = %q, want %q", got, `foo\ bar`)
	}
}

// The word after the subscript gets its separators back. Without clearing the
// counter at the matching `]` the rest of the line would arrive as one word.
func TestTheRestOfTheLineGetsItsSeparatorsBack(t *testing.T) {
	f, err := syntax.Parse("m[foo bar]=v printf one two", subscriptSeparatorGrammar())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cmd := onlySimple(t, f)
	if len(cmd.Assigns) != 1 || len(cmd.Args) != 3 {
		t.Fatalf("%d assignments and %d words, want 1 and 3",
			len(cmd.Assigns), len(cmd.Args))
	}
}

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

// The same flag, the other position: an element of a compound array literal
// that *opens* with `[` carries its subscript through the blanks inside it
// (#2299).
//
// `m=( [two words]=2 )` stored the key `[two` with the value `words]=2`,
// which is two fields joined back into one element rather than a refusal —
// so nothing failed and the array simply held something nobody wrote. It is
// the sibling of the command-position bug above and not a second one: the
// depth counter, the fallback and the probe are all the same, and only the
// question "may a bracket open a subscript here" is new.
//
// Measured 2026-09-13 on bash 5.3.15, that build invoked as `sh`, and
// ksh93u+, each read back through the keys: the element runs to the matching
// `]`. bash 3.2.57 does *not* span in this position although it spans at
// command position, and zsh 5.9.2 globs the bracket instead — which is why
// the rows below name the flag and the dialect packages carry the binaries.

// arrayLiteralElems parses one compound assignment and hands back its
// elements, printed. Written once because every row below wants the same
// three sentences of unwrapping.
func arrayLiteralElems(t *testing.T, src string, d syntax.Dialect) []string {
	t.Helper()
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	cmd := onlySimple(t, f)
	if len(cmd.Assigns) != 1 {
		t.Fatalf("%q: %d assignments, want 1", src, len(cmd.Assigns))
	}
	a := cmd.Assigns[0]
	if !a.IsArray {
		t.Fatalf("%q: not read as an array literal", src)
	}
	out := make([]string, 0, len(a.Elems))
	for _, e := range a.Elems {
		out = append(out, syntax.PrintWord(e))
	}
	return out
}

func TestAnArrayLiteralElementHoldsItsSubscriptsSeparators(t *testing.T) {
	d := subscriptSeparatorGrammar()
	d.ArrayLiteral = true
	for _, c := range []struct {
		name, src string
		want      []string
	}{
		{
			// The issue's own shape.
			name: "a blank", src: "m=( [two words]=2 )",
			want: []string{`[two\ words]=2`},
		},
		{
			// A run is not collapsed, the same way it is not at command
			// position: the subscript is the text between the brackets.
			name: "a run of blanks", src: "m=( [two  words]=2 )",
			want: []string{`[two\ \ words]=2`},
		},
		{
			name: "a tab", src: "m=( [two\twords]=2 )",
			want: []string{"[two\\\twords]=2"},
		},
		{
			// Beside an ordinary element, so the span ends where the
			// bracket does rather than eating the rest of the literal.
			name: "beside a plain element", src: "m=( [one]=1 [two words]=2 )",
			want: []string{"[one]=1", `[two\ words]=2`},
		},
		{
			// The value after the subscript is still cut at the blank —
			// only the bracketed text spans.
			name: "the value is not spanned", src: "m=( [two words]=a b )",
			want: []string{`[two\ words]=a`, "b"},
		},
		{
			name: "the append spelling", src: `m=( [two words]+=2 )`,
			want: []string{`[two\ words]+=2`},
		},
		{
			name: "a nested bracket", src: "m=( [a [b] c]=v )",
			want: []string{`[a\ [b]\ c]=v`},
		},
		{
			// A newline between the brackets is a character of the key, and
			// the element spans the line.
			name: "a newline", src: "m=( [a\nb]=v )",
			want: []string{"[a'\n'b]=v"},
		},
		{
			// A `]` a quote or a backslash protects closes nothing, which is
			// what says the depth is counted over unquoted literal text.
			name: "a quoted close bracket", src: `m=( ['a]b' c]=v )`,
			want: []string{`['a]b'\ c]=v`},
		},
		{
			name: "an escaped close bracket", src: `m=( [a\] b]=v )`,
			want: []string{`[a\]\ b]=v`},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := arrayLiteralElems(t, c.src, d)
			if len(got) != len(c.want) {
				t.Fatalf("%d elements %q, want %d %q",
					len(got), got, len(c.want), c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("element %d = %q, want %q", i, got[i], c.want[i])
				}
			}
		})
	}
}

// The bracket has to be the *front* of the element, and that is measured
// rather than assumed: bash 5.3.15 makes two fields of `a=( pre[1 2]=x )`
// and of `a=( x[1 2] )`, and one field with no subscript at all of
// `a=( "[1 2]"=x )`. Without these rows the flag would read as "a bracket
// inside a literal spans", which is a larger claim than the shell makes.
func TestOnlyAnElementsOwnFrontOpensASpanningSubscript(t *testing.T) {
	d := subscriptSeparatorGrammar()
	d.ArrayLiteral = true
	for _, c := range []struct {
		src  string
		want []string
	}{
		{"a=( pre[1 2]=x )", []string{"pre[1", "2]=x"}},
		{"a=( x[1 2] )", []string{"x[1", "2]"}},
		{`a=( "[1 2]"=x )`, []string{`"[1 2]"=x`}},
		{`a=( \[1 2]=x )`, []string{`\[1`, "2]=x"}},
	} {
		got := arrayLiteralElems(t, c.src, d)
		if len(got) != len(c.want) {
			t.Errorf("%q: %d elements %q, want %d %q",
				c.src, len(got), got, len(c.want), c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%q: element %d = %q, want %q", c.src, i, got[i], c.want[i])
			}
		}
	}
}

// Without the flag the blank still ends the element, which is what zsh makes
// of the same text and what this grammar answers by default. The row is here
// so the flag cannot be deleted and leave the tests green.
func TestWithoutTheFlagAnElementEndsAtTheBlank(t *testing.T) {
	d := syntax.Core()
	d.ArrayLiteral = true
	got := arrayLiteralElems(t, "m=( [two words]=2 )", d)
	want := []string{"[two", "words]=2"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("elements = %q, want %q", got, want)
	}
}

// A bracket with no matching `]` falls back to the ordinary reading here too,
// for the reason the command-position row gives: bash refuses the text and
// ksh93 does something else again, so there is nothing common to implement
// and falling back keeps the flag additive.
func TestAnUnmatchedBracketInALiteralFallsBackToo(t *testing.T) {
	d := subscriptSeparatorGrammar()
	d.ArrayLiteral = true
	got := arrayLiteralElems(t, "a=( [1 2 )", d)
	want := []string{"[1", "2"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("elements = %q, want %q", got, want)
	}
}

// The words after the literal get their separators back: the counter is
// cleared at the start of every word, so a subscript in an element cannot
// reach the command that follows.
func TestTheLineAfterTheLiteralGetsItsSeparatorsBack(t *testing.T) {
	d := subscriptSeparatorGrammar()
	d.ArrayLiteral = true
	f, err := syntax.Parse("m=( [two words]=2 ) printf one two", d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cmd := onlySimple(t, f)
	if len(cmd.Assigns) != 1 || len(cmd.Args) != 3 {
		t.Fatalf("%d assignments and %d words, want 1 and 3",
			len(cmd.Assigns), len(cmd.Args))
	}
}

// And the *position* is given back with them, which is a different claim and
// needs a bracket to say it: a bare `[` opens a spanning subscript between an
// array literal's parentheses and nowhere else, so a statement after the
// literal is cut at the blank exactly as one before it would be.
//
// A mutation run is what asked for this row. Leaving the flag set after the
// literal broke nothing anything else here looks at, because every other row
// after a literal begins with an ordinary word.
func TestTheStatementAfterTheLiteralIsNotElementPosition(t *testing.T) {
	d := subscriptSeparatorGrammar()
	d.ArrayLiteral = true
	f, err := syntax.Parse("m=( [a]=1 )\n[x y]=2", d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(f.Stmts) != 2 {
		t.Fatalf("%d statements, want 2", len(f.Stmts))
	}
	pipe, ok := f.Stmts[1].Expr.(*syntax.Pipeline)
	if !ok || len(pipe.Cmds) != 1 {
		t.Fatalf("second statement is %T, want a one-command pipeline", f.Stmts[1].Expr)
	}
	cmd, ok := pipe.Cmds[0].(*syntax.SimpleCmd)
	if !ok {
		t.Fatalf("second command is %T, want a simple command", pipe.Cmds[0])
	}
	if len(cmd.Args)+len(cmd.Assigns) != 2 {
		t.Errorf("%d words and %d assignments, want the blank to have cut it",
			len(cmd.Args), len(cmd.Assigns))
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// counted is the grammar that reads a repetition count in front of a pattern
// group, which is ksh93's. [Dialect.ExtendedPattern] is set beside it because
// the shell that has one has the other, and the pair is what a caller gets;
// the row below that turns only the count off is what says the two are
// separate flags rather than one.
func countedDialect() Dialect {
	d := Core()
	d.ExtendedPattern, d.ExtendedPatternInCondition = true, true
	d.CountedPatternGroup = true
	return d
}

// A `{…}` in front of a `(` takes the parenthesis into the word.
//
// This is the grammar half of #4931 and it has to land before anything a
// matcher does: the `(` behind a `}` ends the word in every other reading, so
// `f {2,3}(a)` is a parse error and no amount of work on a pattern would ever
// see the text.
//
// Measured 2026-09-27 on `/bin/ksh` `Version AJM 93u+ 2012-08-01` —
// `go version -m` says *not a Go executable* — each probe under
// `env -i PATH=/usr/bin:/bin`.
func TestACountTakesTheParenthesisIntoTheWord(t *testing.T) {
	t.Parallel()
	counted, ext := countedDialect(), Core()
	ext.ExtendedPattern, ext.ExtendedPatternInCondition = true, true

	for _, src := range []string{
		`echo {2,3}(a)`,
		`echo {2}(a)`,
		`echo {2,}(a)`,
		`echo {,2}(a)`,
		// The brace's contents are **not** what decides it. `{z,y}` is a
		// perfectly good list and in front of a `(` it is read here too —
		// what it then matches is the matcher's question, and #4933 is the
		// row that says an implementation keyed on the contents is wrong.
		`echo {z,y}(a)`,
		`echo {a}(b)`,
		`echo {-1,2}(a)`,
		// Mid-word, and behind a group of its own.
		`echo x{2,3}(a)`,
		`echo {2}(a){2}(a)`,
		// And in the two surfaces a pattern has besides a word.
		`case aaa in {2,3}(a)) :;; esac`,
		`[[ aaa == {2,3}(a) ]]`,
	} {
		mustParse(t, src, counted, "a count in front of a group where the dialect has them")
		mustFail(t, src, ext, "the same text with only quantified groups")
		mustFail(t, src, Core(), "the same text in the core grammar")
	}
}

// Which brace takes the parenthesis, and which does not.
//
// Every row here is a refusal in the reference too, so this is the shape of
// the rule rather than a narrowing chosen here. The rows are what keep
// [Lexer.writtenBraceEndsAt] from being "is the previous byte a `}`".
func TestOnlyAWrittenUnquotedBraceTakesTheParenthesis(t *testing.T) {
	t.Parallel()
	counted := countedDialect()

	// Quoted: `f "{2,3}"(a)` is the parenthesis reported unexpected on
	// ksh93u+, the same refusal a shell without the construct gives, and an
	// implementation that answered "this brace is not a count, so expand it"
	// would parse this while getting the rows above right.
	mustFail(t, `echo "{2,3}"(a)`, counted, "a quoted brace")
	mustFail(t, `echo '{2,3}'(a)`, counted, "a single-quoted brace")

	// The brace's own `}` and not merely a `}`: `v='{2,3}'; f $v(a)` is that
	// refusal too, and so is a parameter expansion's closer.
	mustFail(t, `v=1; echo $v(a)`, counted, "no brace at all")
	mustFail(t, `v=1; echo ${v}(a)`, counted, "an expansion's closing brace")

	// Non-empty: `echo A{}(a)B` is a syntax error there where
	// `echo A{,}(a)B` is the word, which is the row that makes the width
	// test `>` rather than `>=`.
	mustFail(t, `echo A{}(a)B`, counted, "an empty brace")
	mustParse(t, `echo A{,}(a)B`, counted, "a count with both ends omitted")

	// The next character: `f {2,3}x(a)` is the refusal, so it is the brace
	// standing immediately in front of the parenthesis.
	mustFail(t, `echo {2,3}x(a)`, counted, "a character between the brace and the parenthesis")

	// And an escaped parenthesis is not one: `f {2,3}\(a\)` is the two words
	// `2(a)` and `3(a)` there, so the brace is an ordinary list.
	mustParse(t, `echo {2,3}\(a\)`, counted, "an escaped parenthesis")
}

// The whole `{…}(…)` is one word, which is what the interpreter then reads as
// a pattern. A group that ended the word would leave the text in two.
func TestACountedGroupIsOneWord(t *testing.T) {
	t.Parallel()
	f, err := Parse("echo {2,3}(a|b)x\n", countedDialect())
	if err != nil {
		t.Fatal(err)
	}
	cmd := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	var lits []string
	for _, w := range cmd.Args {
		var b []byte
		for _, s := range w.Spans {
			b = append(b, s.Value...)
		}
		lits = append(lits, string(b))
	}
	want := []string{"echo", "{2,3}(a|b)x"}
	if len(lits) != len(want) || lits[0] != want[0] || lits[1] != want[1] {
		t.Errorf("words = %q, want %q", lits, want)
	}
}

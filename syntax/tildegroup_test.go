// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A `(` straight after a `~` belongs to the word in one grammar and ends it in
// every other. That is the whole of what the parser has to know about ksh93's
// `~(…)` prefix — which letters mean what is the matcher's question — and it
// is a grammar flag rather than an axis because the other four dialects have
// no reading of the construct to disagree with: they refuse the paren.
func TestATildeGroupBelongsToTheWord(t *testing.T) {
	t.Parallel()
	group := Dialect{TildeGroup: true, DoubleBracket: true}
	plain := Dialect{DoubleBracket: true}
	for _, src := range []string{
		`echo ~(E)abc`,
		`echo a~(x)b`,
		`x=~(Z)abc`,
		`[[ abc == ~(E)a.c ]]`,
		`case abc in ~(E)^a.c$) echo yes;; esac`,
	} {
		mustParse(t, src, group, "a `(` after a `~` belongs to the word here")
		mustFail(t, src, plain, "a `(` after a `~` ends the word here")
	}
}

// The group is taken whole, nesting and all, and the text it holds reaches the
// matcher as written — the same contract scanPatternGroup already has for a
// pattern group.
func TestATildeGroupIsOneWord(t *testing.T) {
	t.Parallel()
	d := Dialect{TildeGroup: true}
	for _, c := range []struct{ src, want string }{
		{`echo ~(E)abc`, "~(E)abc"},
		{`echo ~()abc`, "~()abc"},
		{`echo ~(Elr)a.c`, "~(Elr)a.c"},
		{`echo a~(x)b`, "a~(x)b"},
	} {
		f, err := Parse(c.src, d)
		if err != nil {
			t.Fatalf("%q: %v", c.src, err)
		}
		pipe, ok := f.Stmts[0].Expr.(*Pipeline)
		if !ok {
			t.Fatalf("%q: not a pipeline", c.src)
		}
		cmd, ok := pipe.Cmds[0].(*SimpleCmd)
		if !ok || len(cmd.Args) != 2 {
			t.Fatalf("%q: want one argument, got %#v", c.src, cmd)
		}
		if got := cmd.Args[1].Literal(); got != c.want {
			t.Errorf("%q: argument %q, want %q", c.src, got, c.want)
		}
	}
}

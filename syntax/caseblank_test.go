// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A blank inside a `case` arm's parenthesized pattern list (#1744).
//
// The same grammar the newline has and the same one dialect: inside the
// parentheses the list is one alternation word, so a blank between two pieces
// of pattern is a character of the pattern rather than the end of a word.
// Measured on zsh 5.9.2, 2026-09-10 — `case 'a b' in (a b) echo hit;; (*) echo
// no;; esac` is `hit` there and a syntax error in bash 5.3.15, that build
// invoked as `sh`, bash 3.2.57, ksh93u+ and dash.
//
// The rows are written as *patterns* rather than as "it parsed", because
// parsing is the cheap half. A reading that admitted the line and then dropped
// or collapsed the blanks would parse every row here and match none of them,
// which is why `(a  b)` and `(a<tab>b)` are separate rows from `(a b)`: the
// pattern is the source text.

// caseBlankGrammar is the grammar this construct needs, named by the construct.
func caseBlankGrammar(d *Dialect) {
	d.CasePatternListSpansBlanks = true
	// A bare `(` belonging to a word is two flags of its own, and two rows
	// below need them: the line this construct exists for is a *group*
	// followed by a blank. One says a group may open inside a word and the
	// other that it may open at the front of one. The test names the flags
	// rather than a shell that happens to have all three.
	d.PatternAlternation = true
	d.GlobQualifiers = true
}

func TestABlankInAParenthesizedCasePatternList(t *testing.T) {
	d := Core()
	caseBlankGrammar(&d)
	for _, tc := range []struct {
		name, src string
		want      []string
	}{
		{
			// The issue's own shape.
			"between two words",
			"case x in (a b) echo m;; esac",
			[]string{"a b"},
		},
		{
			// A run is not collapsed, which is what says the pattern is the
			// text and not a join of the words with one space.
			"a run of them",
			"case x in (a  b) echo m;; esac",
			[]string{"a  b"},
		},
		{
			// And a tab is not a space, for the same reason.
			"a tab",
			"case x in (a\tb) echo m;; esac",
			[]string{"a\tb"},
		},
		{
			"three words",
			"case x in (a b c) echo m;; esac",
			[]string{"a b c"},
		},
		{
			// Trailing blanks are still a separator: the pattern ends where
			// the list does.
			"before the closing paren",
			"case x in (a b ) echo m;; esac",
			[]string{"a b"},
		},
		{
			"and leading ones after the opening paren",
			"case x in ( a b) echo m;; esac",
			[]string{"a b"},
		},
		{
			// The `|` still separates whole patterns, which is what keeps
			// `(a | b)` two alternatives everywhere.
			"around the separator",
			"case x in (a | b) echo m;; esac",
			[]string{"a", "b"},
		},
		{
			"held on one side of it only",
			"case x in (a b|z) echo m;; esac",
			[]string{"a b", "z"},
		},
		{
			"and on the other",
			"case x in (z|a b) echo m;; esac",
			[]string{"z", "a b"},
		},
		{
			// The failing line's own shape: a group, a blank, more pattern.
			"after a group",
			"case x in ((x) y) echo m;; esac",
			[]string{"(x) y"},
		},
		{
			"and before one",
			"case x in (y (x)) echo m;; esac",
			[]string{"y (x)"},
		},
		{
			// Quoting is not what does this — the blank is bare — but a
			// quoted one lands in the same pattern.
			"beside a quoted word",
			`case x in ("a b" c) echo m;; esac`,
			[]string{"a b c"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := firstArm(t, tc.src, d)
			if len(got) != len(tc.want) {
				t.Fatalf("%d patterns %q, want %d %q", len(got), got, len(tc.want), tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("pattern %d is %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// Where the two flags meet, a blank beside a newline is text as well.
//
// Measured: `case $'a \n b' in (a` blank newline blank `b)` matches there and
// `case $'a\nb'` does not, so neither the blanks nor the newline is dropped
// and none of them is folded into another.
func TestABlankBesideANewlineIsTextToo(t *testing.T) {
	d := Core()
	caseBlankGrammar(&d)
	d.CasePatternListSpansNewlines = true
	got := firstArm(t, "case x in (a \n b) echo m;; esac", d)
	if len(got) != 1 || got[0] != "a \n b" {
		t.Errorf("patterns %q, want [%q]", got, "a \n b")
	}
}

// Without the flag every one of those is a parse error, which is five of the
// six shells' answer and this grammar's default.
func TestABlankInACasePatternListIsRefusedWithoutTheFlag(t *testing.T) {
	for _, src := range []string{
		"case x in (a b) echo m;; esac",
		"case x in (a  b) echo m;; esac",
		"case x in (a\tb) echo m;; esac",
		"case x in ((x) y) echo m;; esac",
		"case x in (a b|z) echo m;; esac",
	} {
		if _, err := Parse(src, Core()); err == nil {
			t.Errorf("%q parsed, want a syntax error without the flag", src)
		}
	}
}

// The paren is what opens it, and the position is not.
//
// An arm written *without* the paren is a parse error in the dialect that has
// the flag too — measured, `case 'a b' in a b) …` is “parse error near `b'“
// there — so a blank is only text between the paren the arm carries and the
// one that closes it. Without this row the flag would read as "a word may
// follow a pattern", which is a different and much larger claim.
func TestOnlyAParenthesizedListTakesTheBlank(t *testing.T) {
	d := Core()
	caseBlankGrammar(&d)
	if _, err := Parse("case x in a b) echo m;; esac", d); err == nil {
		t.Error("an arm with no paren parsed, want a syntax error even with the flag")
	}
}

// An operator after the blanks is still an operator, so they separate there
// too: `(a >b)` is “parse error near `>'“ in the shell that has the flag,
// and a reading that swallowed the blank would have made the pattern `a ` and
// blamed the `)` instead.
func TestAnOperatorAfterTheBlankStillEndsThePattern(t *testing.T) {
	d := Core()
	caseBlankGrammar(&d)
	for _, src := range []string{
		"case x in (a >b) echo m;; esac",
		"case x in (a ;b) echo m;; esac",
		"case x in (a &b) echo m;; esac",
	} {
		if _, err := Parse(src, d); err == nil {
			t.Errorf("%q parsed, want a syntax error", src)
		}
	}
}

// And the arm's *body* is ordinary commands again, where a blank separates
// words. Without clearing the flag at the closing paren the body's first
// command would arrive as one word.
func TestTheArmsBodyGetsItsBlanksBack(t *testing.T) {
	d := Core()
	caseBlankGrammar(&d)
	f, err := Parse("case x in (a b) echo one two;; esac", d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	c := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*CaseClause)
	cmd, ok := c.Items[0].Body[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	if !ok {
		t.Fatalf("the body's first command is %T, want a simple command", c.Items[0].Body[0].Expr)
	}
	if len(cmd.Args) != 3 {
		t.Fatalf("%d words %v, want 3 — the body's blanks were taken as text", len(cmd.Args), cmd.Args)
	}
	if got := cmd.Args[2].Literal(); got != "two" {
		t.Errorf("the third word is %q, want %q", got, "two")
	}
}

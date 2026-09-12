// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// A newline inside a `case` arm's parenthesized pattern list.
//
// One dialect reads the list as a single alternation word, so a newline in it
// is a character of the pattern rather than the end of a word or of a
// statement. Measured 2026-09-07 over a script file with a scratch HOME and
// ZDOTDIR: zsh 5.9.2 runs the arm, and bash 5.3.15, that build invoked as
// `sh`, bash 3.2.57, dash and ksh93u+ all refuse the line — three wordings and
// three statuses.
//
// The rows are written as *patterns* rather than as "it parsed", because
// parsing is the cheap half: what the newline does is join a pattern, and only
// the pattern's text says which one it joined.

// caseGrammar is the grammar this construct needs, named by the construct.
func caseGrammar(d *Dialect) {
	d.CasePatternListSpansNewlines = true
	// The empty alternative is a flag of its own (#1083) and one row below
	// needs it, so the test names it rather than a shell that has both.
	d.CasePatternMayBeEmpty = true
}

// firstArm is the pattern list of the first arm of the first `case` in src.
func firstArm(t *testing.T, src string, d Dialect) []string {
	t.Helper()
	f, err := Parse(src, d)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	c, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*CaseClause)
	if !ok {
		t.Fatalf("%s: first command is %T, want a case clause", src, f.Stmts[0].Expr.(*Pipeline).Cmds[0])
	}
	out := make([]string, 0, len(c.Items[0].Patterns))
	for _, w := range c.Items[0].Patterns {
		var b strings.Builder
		for _, s := range w.Spans {
			b.WriteString(s.Value)
		}
		out = append(out, b.String())
	}
	return out
}

func TestANewlineInAParenthesizedCasePatternList(t *testing.T) {
	d := Core()
	caseGrammar(&d)
	for _, tc := range []struct {
		name, src string
		want      []string
	}{
		{
			// The issue's own shape: the newline joins the *second*
			// alternative, which is why the subject that matches it is a
			// value beginning with a newline.
			"after the separator",
			"case a in (a|\nb) echo m;; esac",
			[]string{"a", "\nb"},
		},
		{
			// And before it, which is what says the rule is not about the
			// `|` at all: here the newline joins the *first* alternative, so
			// a subject of `a` matches neither.
			"before the separator",
			"case a in (a\n|b) echo m;; esac",
			[]string{"a\n", "b"},
		},
		{
			"with no separator in the list at all",
			"case a in (a\n) echo m;; esac",
			[]string{"a\n"},
		},
		{
			"two of them",
			"case a in (a|\n\nb) echo m;; esac",
			[]string{"a", "\n\nb"},
		},
		{
			// The empty alternative this dialect also has, and the two do
			// not interfere: the emptiness is still read off the separator.
			"beside an empty alternative",
			"case a in (|a|\nb) echo m;; esac",
			[]string{"", "a", "\nb"},
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

// Without the flag every one of those is a parse error, which is five of the
// six shells' answer and this grammar's default.
func TestANewlineInACasePatternListIsRefusedWithoutTheFlag(t *testing.T) {
	for _, src := range []string{
		"case a in (a|\nb) echo m;; esac",
		"case a in (a\n|b) echo m;; esac",
		"case a in (a\n) echo m;; esac",
	} {
		if _, err := Parse(src, Core()); err == nil {
			t.Errorf("%q parsed, want a syntax error without the flag", src)
		}
	}
}

// The paren is what opens it, and the position is not.
//
// An arm written *without* the paren is a parse error in the dialect that has
// the flag too — measured, `case a in a|` newline `b)` is `parse error near
// newline` there — so a newline is only text between the paren the arm carries
// and the one that closes it.
func TestOnlyAParenthesizedListTakesTheNewline(t *testing.T) {
	d := Core()
	caseGrammar(&d)
	if _, err := Parse("case a in a|\nb) echo m;; esac", d); err == nil {
		t.Error("an arm with no paren parsed, want a syntax error even with the flag")
	}
}

// And the arm's *body* is ordinary commands again, where a newline separates
// statements. Without clearing the flag at the closing paren, the body's first
// line would swallow the rest of the arm.
func TestTheArmsBodyGetsItsNewlinesBack(t *testing.T) {
	d := Core()
	caseGrammar(&d)
	f, err := Parse("case a in (a|\nb)\necho one\necho two\n;; esac", d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	c := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*CaseClause)
	if n := len(c.Items[0].Body); n != 2 {
		t.Fatalf("the arm's body is %d statements, want 2 — the newlines after the list separate again", n)
	}
	// And the *first* statement is the whole of the first line, which
	// counting the statements alone does not say: with the flag still in
	// force for one token more, the newline after the `)` joins the command
	// name, and two statements come back with the wrong name on the first.
	sc, ok := c.Items[0].Body[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	if !ok {
		t.Fatalf("the arm's first statement is %T, want a simple command", c.Items[0].Body[0].Expr.(*Pipeline).Cmds[0])
	}
	if got := sc.Args[0].Spans[0].Value; got != "echo" {
		t.Errorf("the arm's first command is %q, want %q — the newline after the `)` is a separator again", got, "echo")
	}
}

// A bare newline in a word has to survive being printed, and a backslash is
// the one escape that cannot carry it: a backslash before a newline is a line
// continuation, which the next read removes.
//
// It goes back **bare**, inside the arm's parenthesis — the same treatment a
// bare blank gets there, and for the same reason. Both characters are the
// pattern's, both are ordinary only inside the parentheses, and an *unquoted*
// literal span holding either is itself the evidence that the dialect had the
// rule, since no other grammar here puts one in a word. It is also what the
// shell being modeled writes back (#1254).
//
// It used to go back single-quoted, which kept the character and let the arm
// re-parse under a dialect *without* the rule. That was a real property and
// it is given up on purpose: the blank had already given it up, so keeping it
// for the newline made one construct print two ways, and the quoting was what
// made the paren unsettled — it went in on the first pass and came back out
// on the second.
func TestABareNewlineInAWordSurvivesPrinting(t *testing.T) {
	d := Core()
	caseGrammar(&d)
	// The blank half of the same grammar, which two rows below need: the
	// point of them is that the two characters are one rule now.
	d.CasePatternListSpansBlanks = true
	for _, tc := range []struct {
		src  string
		want []string
		arm  string
	}{
		// The newline alone, and the newline with a blank after it — the
		// pair that says the two characters are one rule now.
		{"case a in (a|\nb) echo m;; esac", []string{"a", "\nb"}, "(a|\nb)"},
		{"case a in (a |\n b) echo m;; esac", []string{"a", "\n b"}, "(a|\n b)"},
		// A blank with no newline, unchanged.
		{"case a in (a b) echo m;; esac", []string{"a b"}, "(a b)"},
	} {
		f, err := Parse(tc.src, d)
		if err != nil {
			t.Fatalf("%q: parse: %v", tc.src, err)
		}
		printed := Print(f)
		if strings.Contains(printed, "\\\n") {
			t.Errorf("%q printed %q, want no backslash-newline — the next read would take it out", tc.src, printed)
		}
		if !strings.Contains(printed, tc.arm) {
			t.Errorf("%q printed %q, want the arm written %q", tc.src, printed, tc.arm)
		}
		// The reparse needs the same dialect, which is the property that
		// replaced the old one: what is promised is the same *tree*.
		got := firstArm(t, printed, d)
		if len(got) != len(tc.want) {
			t.Errorf("reparsed %q as %q, want %q", printed, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("reparsed %q as %q, want %q", printed, got, tc.want)
				break
			}
		}
		// And the print is settled, which is what the paren used not to be.
		again, err := Parse(printed, d)
		if err != nil {
			t.Fatalf("%q printed %q, which does not parse: %v", tc.src, printed, err)
		}
		if twice := Print(again); twice != printed {
			t.Errorf("%q is not settled:\n  once:  %q\n  twice: %q", tc.src, printed, twice)
		}
	}
}

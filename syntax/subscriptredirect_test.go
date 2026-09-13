// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// A redirection's target holds a subscript's separators too, in the one
// dialect that reads it that way (#2449).
//
// Measured 2026-09-13 on ksh93u+ 2012-08-01 against bash 5.3.15, bash 3.2.57
// and bash as `sh`, each run in an empty directory and read back with `ls`:
// `> m[foo bar] echo hi` writes one file called `m[foo bar]` in ksh93 and
// writes `m[foo` in every bash, which then reports `bar]: command not found`.
// So four columns span at command position and only one spans here, which is
// why #2410 followed bash and left this behind.
//
// The rows are written as the redirection's *target text* rather than as "it
// parsed", because a reading that admitted the line and then cut the target
// somewhere else would parse and open the wrong file.

// redirectSubscriptGrammar is the grammar the construct needs. The
// command-position flag is on beside it because the dialect that has this one
// has that one, and the row below with only the first flag is what says the
// two are separate.
func redirectSubscriptGrammar() syntax.Dialect {
	d := syntax.Core()
	d.SubscriptSpansSeparators = true
	d.SubscriptSpansSeparatorsInRedirect = true
	return d
}

// firstRedirect returns the target text of a simple command's only
// redirection, printed from the word rather than taken from the source, so a
// reading that produced the right bytes by a different route still shows.
func firstRedirect(t *testing.T, src string, d syntax.Dialect) string {
	t.Helper()
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	cmd := onlySimple(t, f)
	if len(cmd.Redirs) != 1 {
		t.Fatalf("%q: %d redirections, want 1", src, len(cmd.Redirs))
	}
	return syntax.PrintWord(cmd.Redirs[0].Word)
}

func TestARedirectionTargetHoldsASubscriptsSeparators(t *testing.T) {
	d := redirectSubscriptGrammar()
	for _, c := range []struct {
		name, src, target string
		args              int
	}{
		{
			// The issue's own shape: the redirection stands in front of the
			// command word, so the target is read where a command may begin.
			name: "a prefix", src: "> m[foo bar] echo hi",
			target: `m[foo\ bar]`, args: 2,
		},
		{
			// An IO number in front of the operator changes nothing.
			name: "an io number", src: "2> m[foo bar] echo hi",
			target: `m[foo\ bar]`, args: 2,
		},
		{
			// Not blanks alone, the same way the command-position rows are
			// not: every separator inside the brackets is text.
			name: "a semicolon", src: "> m[a; b] echo hi",
			target: `m[a\;\ b]`, args: 2,
		},
		{
			// Brackets nest here too, so the *matching* `]` ends the
			// subscript.
			name: "a nested bracket", src: "> m[a [b] c] echo hi",
			target: `m[a\ [b]\ c]`, args: 2,
		},
		{
			// The bracket ends the subscript and not the word: measured,
			// `> pre[1 2]post echo hi` names one file `pre[1 2]post`.
			name: "text after the close bracket", src: "> pre[1 2]post echo hi",
			target: `pre[1\ 2]post`, args: 2,
		},
		{
			// A quoted `]` closes nothing, which is what says the depth is
			// counted over unquoted literal text alone.
			name: "a quoted close bracket", src: `> m['a]b' c] echo hi`,
			target: `m['a]b'\ c]`, args: 2,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := firstRedirect(t, c.src, d); got != c.target {
				t.Errorf("target = %q, want %q", got, c.target)
			}
		})
	}
}

// A redirection *after* a word is argument position, and ksh93 splits there:
// `echo hi > m[foo bar]` writes `m[foo` and hands `bar]` to echo, in every
// column including the one that spans in front of the command. Without this
// row the flag would read as "a redirection target spans", which is a larger
// claim than the shell makes.
func TestARedirectionAfterAWordStillEndsAtTheBlank(t *testing.T) {
	f, err := syntax.Parse("echo hi > m[foo bar]", redirectSubscriptGrammar())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cmd := onlySimple(t, f)
	if len(cmd.Redirs) != 1 {
		t.Fatalf("%d redirections, want 1", len(cmd.Redirs))
	}
	if got := syntax.PrintWord(cmd.Redirs[0].Word); got != "m[foo" {
		t.Errorf("target = %q, want %q", got, "m[foo")
	}
	// `bar]` became a word of the command, which is the other half of the
	// same cut and is what the shell prints.
	if len(cmd.Args) != 3 {
		t.Errorf("%d words, want 3", len(cmd.Args))
	}
}

// A compound command's trailing redirection is not an argument, and it spans:
// measured, `{ :; } > m[foo bar]` writes one file in ksh93 while every bash
// refuses the line outright. This is the row that says the condition is "the
// target stands where a command may begin" rather than "the redirection came
// first".
func TestACompoundCommandsRedirectionTargetSpans(t *testing.T) {
	for _, src := range []string{
		"{ :; } > m[foo bar]",
		"for i in x; do :; done > m[foo bar]",
	} {
		f, err := syntax.Parse(src, redirectSubscriptGrammar())
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		if len(f.Stmts) != 1 {
			t.Fatalf("%q: %d statements, want 1", src, len(f.Stmts))
		}
		pipe, ok := f.Stmts[0].Expr.(*syntax.Pipeline)
		if !ok || len(pipe.Cmds) != 1 {
			t.Fatalf("%q: %T, want a one-command pipeline", src, f.Stmts[0].Expr)
		}
		got := ""
		switch c := pipe.Cmds[0].(type) {
		case *syntax.Group:
			if len(c.Redirs) == 1 {
				got = syntax.PrintWord(c.Redirs[0].Word)
			}
		case *syntax.ForClause:
			if len(c.Redirs) == 1 {
				got = syntax.PrintWord(c.Redirs[0].Word)
			}
		default:
			t.Fatalf("%q: %T, want a compound command", src, c)
		}
		if got != `m[foo\ bar]` {
			t.Errorf("%q: target = %q, want %q", src, got, `m[foo\ bar]`)
		}
	}
}

// A `case` subject is the near miss, and it is why the position has a flag of
// its own rather than reusing the one that says no assignment may stand here.
// It is read where a command may begin and takes no assignment, exactly as a
// redirection prefix does — and ksh93 **splits** there:
// `case m[foo bar] in *) ;; esac` is the message `bar]' unexpected.
func TestACaseSubjectStillEndsAtTheBlank(t *testing.T) {
	if _, err := syntax.Parse("case m[foo bar] in *) echo arm;; esac",
		redirectSubscriptGrammar()); err == nil {
		t.Fatal("parsed, want the blank to have cut the subject and the `]` to be unexpected")
	}
}

// The name-in-front condition is the command-position one and is not restated
// here: measured, `> 1m[foo bar]`, `> m-n[foo bar]` and `> [foo bar]` all
// still end at the blank in ksh93.
func TestOnlyANameOpensASpanningRedirectionSubscript(t *testing.T) {
	d := redirectSubscriptGrammar()
	for _, c := range []struct{ src, target string }{
		{"> 1m[foo bar] echo hi", "1m[foo"},
		{"> m-n[foo bar] echo hi", "m-n[foo"},
	} {
		if got := firstRedirect(t, c.src, d); got != c.target {
			t.Errorf("%q: target = %q, want %q", c.src, got, c.target)
		}
	}
}

// Additive on the same terms as the command-position flag: with no matching
// `]` the word is left exactly as a grammar without the flag reads it. ksh93
// swallows the rest of the input there and bash refuses, so there is no
// common answer — and nothing that parses today parses differently with the
// flag on.
func TestARedirectionSubscriptWithNoCloseIsLeftAlone(t *testing.T) {
	if got := firstRedirect(t, "> m[foo bar echo hi", redirectSubscriptGrammar()); got != "m[foo" {
		t.Errorf("target = %q, want %q", got, "m[foo")
	}
}

// Without the redirection flag the target is cut at the blank even where the
// command-position flag is on, which is the reading every bash has. The row
// is here so the new flag cannot be deleted and leave the tests green.
func TestWithoutTheRedirectionFlagTheTargetEndsAtTheBlank(t *testing.T) {
	d := syntax.Core()
	d.SubscriptSpansSeparators = true
	if got := firstRedirect(t, "> m[foo bar] echo hi", d); got != "m[foo" {
		t.Errorf("target = %q, want %q", got, "m[foo")
	}
}

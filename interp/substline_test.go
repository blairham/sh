// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A command substitution's body is parsed on its own, so its positions count
// from one. The script it was written in did not start there.
//
// Reported at line 1 before this: every message from inside a `$( … )` named
// the top of the file. Found by the run sweep on a script whose line 22 is
// `GRANTED_OUTPUT=$(assumego "$@")` — reported at line 1, twenty-one lines
// from where a reader would look.
//
// Unanimous across the panel for `$( … )`, so it is the core's behavior. The
// one shape the panel splits on has a test of its own below.
func TestASubstitutionsBodyIsPlacedInTheScript(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		want string
	}{
		{"in an assignment", "true\ntrue\ntrue\nx=$(nosuchcmd)\n", ":4:"},
		{"in an argument", "true\ntrue\ntrue\necho $(nosuchcmd)\n", ":4:"},
		{"backquoted", "true\ntrue\ntrue\nx=`nosuchcmd`\n", ":4:"},
		{"one inside another", "true\ntrue\ntrue\nx=$(echo $(nosuchcmd))\n", ":4:"},
		{"backquoted inside one", "true\ntrue\ntrue\nx=$(echo `nosuchcmd`)\n", ":4:"},
		{
			// The body's own lines count from where it opened, so a command
			// two lines into a substitution that opened on line 2 is on
			// line 4 and not on line 2.
			"a body spanning lines", "true\nx=$(\necho hi\nnosuchcmd\n)\n", ":4:",
		},
		{"a body spanning more", "true\nx=$(\necho a\necho b\nnosuchcmd\n)\n", ":5:"},
		{"inside a compound command", "true\nif true\nthen\ntrue\nx=$(nosuchcmd)\nfi\n", ":5:"},
		{"inside a function", "f() {\n  true\n  x=$(nosuchcmd)\n}\ntrue\nf\n", ":3:"},
		// Shapes that were already right, so the offset must not move them.
		{"no substitution at all", "true\ntrue\ntrue\nnosuchcmd\n", ":4:"},
		{"a substitution that runs cleanly", "x=$(echo hi)\ntrue\nnosuchcmd\n", ":3:"},
		{"the first line", "x=$(nosuchcmd)\n", ":1:"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := substLine(t, c.src, Diagnostics{Location: LocationTightLine}); !strings.Contains(got, c.want) {
				t.Errorf("reported %q, want it to name %s", got, c.want)
			}
		})
	}
}

// One dialect counts a backquoted body from one and its `$( … )` body from
// the file — two answers for the two spellings of one construct.
func TestABackquotedBodyMayCountFromOne(t *testing.T) {
	dg := Diagnostics{Location: LocationTightLine, BackquotedSubstitutionRestartsLines: true}
	for _, c := range []struct {
		name string
		src  string
		want string
	}{
		{"backquoted restarts", "true\ntrue\ntrue\nx=`nosuchcmd`\n", ":1:"},
		{"and counts its own lines from there", "true\ntrue\nx=`\nnosuchcmd\n`\n", ":2:"},
		// The other spelling is not affected, which is the whole point of
		// the field being about backquotes rather than about substitution.
		{"the other spelling still does not", "true\ntrue\ntrue\nx=$(nosuchcmd)\n", ":4:"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := substLine(t, c.src, dg); !strings.Contains(got, c.want) {
				t.Errorf("reported %q, want it to name %s", got, c.want)
			}
		})
	}
}

// A failing redirect inside a substitution, which reaches the line through a
// different door: the redirect axis sets the line from the redirect's own
// position rather than from the command's, and that position needs placing in
// the script too.
func TestARedirectInsideASubstitutionIsPlacedInTheScript(t *testing.T) {
	dg := Diagnostics{Location: LocationTightLine, RedirectFailureLine: LineOfRedirect}
	// `} < missing` is on line 4 of the file and line 3 of the body.
	src := "true\nx=$(\n{ :\n} < missing\n)\n"
	if got := substLine(t, src, dg); !strings.Contains(got, ":4:") {
		t.Errorf("reported %q, want it to name :4:", got)
	}
}

func substLine(t *testing.T, src string, dg Diagnostics) string {
	t.Helper()
	var errs strings.Builder
	sem := PosixSemantics()
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &dg, Name: "sh",
		Stdout: &strings.Builder{}, Stderr: &errs,
	})
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return errs.String()
}

// The **refusal**'s prefix honors the same answer, which it did not.
//
// The runner's location shifted a backquoted body's runtime complaints and
// the parse failure's prefix did not, so a body that would not parse was
// placed at the script's line while the number *inside* the same sentence —
// where a dialect writes one — counted from the body. One answer, two
// readings of it, and only one row of any corpus could see the difference
// (#2471).
func TestABackquotedBodysRefusalIsPlacedWhereItsFailuresAre(t *testing.T) {
	dg := Diagnostics{
		Location:                            LocationTightLine,
		BackquotedSubstitutionRestartsLines: true,
		SyntaxError:                         "syntax error: unexpected %[1]s",
	}
	src := "true\ntrue\ntrue\nx=`if; then :; fi`\n"
	if got := substLine(t, src, dg); !strings.Contains(got, ":1:") {
		t.Errorf("restarting: reported %q, want the refusal at the body's own line", got)
	}
	// The other spelling is untouched, which is what keeps the answer about
	// backquotes rather than about substitution.
	if got := substLine(t, "true\ntrue\ntrue\nx=$(if; then :; fi)\n", dg); !strings.Contains(got, ":4:") {
		t.Errorf("the other spelling: reported %q, want :4:", got)
	}
	// And a dialect that does not restart them places the refusal in the
	// script, exactly as it always did.
	plain := Diagnostics{Location: LocationTightLine, SyntaxError: "syntax error: unexpected %[1]s"}
	if got := substLine(t, src, plain); !strings.Contains(got, ":4:") {
		t.Errorf("not restarting: reported %q, want :4:", got)
	}
}

// One dialect adds the body's newlines to a refused backquoted body's line,
// and only where the substitution stands in the command's **first token**.
//
// The axis is the addition; the discriminator is the token. Every row below
// holds the same body and the same opening line and they answer three ways,
// which is what says the rule is not an offset — see
// Diagnostics.BackquotedSubstitutionFailureAddsItsBodysNewlines for the sweep
// it was read off.
func TestABackquotedBodysRefusalMayCountTheBodyAgain(t *testing.T) {
	dg := Diagnostics{
		Location:    LocationTightLine,
		SyntaxError: "syntax error: unexpected %[1]s",
		BackquotedSubstitutionFailureAddsItsBodysNewlines: true,
	}
	plain := Diagnostics{Location: LocationTightLine, SyntaxError: "syntax error: unexpected %[1]s"}
	for _, c := range []struct {
		name       string
		src        string
		want, base string
	}{
		{
			// The body opens on line 2 and fails on the file's line 3, so
			// the addition of its one newline names line 4.
			"an assignment that is the first token",
			"true\nv=`echo hi\nif; then :; fi`\n", ":4:", ":3:",
		},
		{
			"a command word ahead of it",
			"true\ncat `echo hi\nif; then :; fi`\n", ":3:", ":3:",
		},
		{
			// The same assignment, with an assignment written before it.
			// Nothing about the body moved and the answer did.
			"an assignment that is not the first token",
			"true\nv=1 w=`echo hi\nif; then :; fi`\n", ":3:", ":3:",
		},
		{
			"a redirection ahead of it",
			"true\n2>&1 `echo hi\nif; then :; fi`\n", ":3:", ":3:",
		},
		{
			// A redirection *behind* it leaves the word first, so the
			// addition stands — the pair is what says this is the token's
			// position and not the presence of a redirection.
			"a redirection behind it",
			"true\n`echo hi\nif; then :; fi` 2>&1\n", ":4:", ":3:",
		},
		{
			// Three lines of body, failing on the second of them: the
			// addition is the body's *whole* newline count, so 3 + 2.
			"a three-line body failing on its second",
			"true\nv=`echo hi\nif; then :; fi\necho t`\n", ":5:", ":3:",
		},
		{
			// The other spelling is untouched, which keeps the answer about
			// backquotes rather than about substitution.
			"the other spelling",
			"true\nv=$(echo hi\nif; then :; fi)\n", ":3:", ":3:",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := substLine(t, c.src, dg); !strings.Contains(got, c.want) {
				t.Errorf("adding: reported %q, want it to name %s", got, c.want)
			}
			// Without the axis every row is the failure's own line, which is
			// what three of the four columns write.
			if got := substLine(t, c.src, plain); !strings.Contains(got, c.base) {
				t.Errorf("plain: reported %q, want it to name %s", got, c.base)
			}
		})
	}
}

// A substitution inside a re-read fragment — an expansion's operand, a
// subscript, an arithmetic expression — is placed on the line that holds it.
//
// The fragment is lexed on its own, so every span it yields is numbered from
// the fragment's first line. That number reached the reader: measured
// 2026-09-19 on a two-line script whose second line is the row, every
// reference column names line 2 and every one of ours named line 1 (#3810).
//
//	echo ${x:-$(echo hi; for)}     zsh 5.9.2   s.sh:2: parse error near `)'
//	                               ours        s.sh:1: parse error near `)'
//
// The ksh93 column is what localized it: its sentence carries the line twice
// — `s.sh: line 2: syntax error at line 1:` — so the prefix, placed from the
// command, was right while the failure's own line was wrong.
//
// Every dialect and every one of those fragments, so a plain fix rather than
// an axis. The rows below use a command that is not found, because that
// reaches the line through the body's own runner; the refusal route is the
// test under it.
func TestASubstitutionInAFragmentIsPlacedInTheScript(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		want string
	}{
		{"a default operand", "true\ntrue\necho ${x:-$(nosuchcmd)}\n", ":3:"},
		{"an alternate operand", "true\ntrue\nx=1; echo ${x:+$(nosuchcmd)}\n", ":3:"},
		{"a pattern operand", "true\ntrue\necho ${x#$(nosuchcmd)}\n", ":3:"},
		{"a replacement operand", "true\ntrue\necho ${x/a/$(nosuchcmd)}\n", ":3:"},
		{"a substring offset", "true\ntrue\necho ${x:$(nosuchcmd)}\n", ":3:"},
		{"a quoted operand", "true\ntrue\necho \"${x:-$(nosuchcmd)}\"\n", ":3:"},
		{"an operand inside an operand", "true\ntrue\necho ${x:-${y:-$(nosuchcmd)}}\n", ":3:"},
		{"an arithmetic expansion", "true\ntrue\necho $(( $(nosuchcmd) + 1 ))\n", ":3:"},
		{"an arithmetic command", "true\ntrue\n(( $(nosuchcmd) + 1 ))\n", ":3:"},
		{"an arithmetic loop header", "true\ntrue\nfor ((i=$(nosuchcmd);0;)); do :; done\n", ":3:"},
		{"an operand inside an arithmetic expansion", "true\ntrue\necho $(( ${y:-$(nosuchcmd)} + 1 ))\n", ":3:"},
		{"an arithmetic expansion inside an operand", "true\ntrue\necho ${x:-$(( $(nosuchcmd) ))}\n", ":3:"},
		// The body's own lines still count from where it opened, which the
		// offset must not flatten.
		{"a body spanning lines in an operand", "true\necho ${x:-$(\ntrue\nnosuchcmd\n)}\n", ":4:"},
		// Shapes that were already right, so the offset must not move them.
		{"the same body outside an operand", "true\ntrue\necho $(nosuchcmd)\n", ":3:"},
		{"an operand on the first line", "echo ${x:-$(nosuchcmd)}\n", ":1:"},
		{"an operand with no substitution", "true\ntrue\nnosuchcmd ${x:-y}\n", ":3:"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := substLine(t, c.src, Diagnostics{Location: LocationTightLine}); !strings.Contains(got, c.want) {
				t.Errorf("reported %q, want it to name %s", got, c.want)
			}
		})
	}
}

// And the **refusal** of such a body, which is the route the issue was filed
// from: the body never parses, so nothing of it runs and the line comes from
// the refusal rather than from the body's runner.
func TestARefusedBodyInAFragmentIsPlacedInTheScript(t *testing.T) {
	dg := Diagnostics{Location: LocationTightLine, SyntaxError: "syntax error: unexpected %[1]s"}
	for _, c := range []struct {
		name string
		src  string
		want string
	}{
		{"a default operand", "true\ntrue\necho ${x:-$(if; then :; fi)}\n", ":3:"},
		{"a pattern operand", "true\ntrue\necho ${x#$(if; then :; fi)}\n", ":3:"},
		{"an arithmetic expansion", "true\ntrue\necho $(( $(if; then :; fi) ))\n", ":3:"},
		{"an arithmetic command", "true\ntrue\n(( $(if; then :; fi) ))\n", ":3:"},
		{"an arithmetic loop header", "true\ntrue\nfor ((i=$(if; then :; fi);0;)); do :; done\n", ":3:"},
		// Already right, and still right.
		{"outside a fragment", "true\ntrue\necho $(if; then :; fi)\n", ":3:"},
		{"on the first line", "echo ${x:-$(if; then :; fi)}\n", ":1:"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := substLine(t, c.src, dg); !strings.Contains(got, c.want) {
				t.Errorf("reported %q, want it to name %s", got, c.want)
			}
		})
	}
}

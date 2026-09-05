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

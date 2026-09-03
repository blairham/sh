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

// Which line a failed open is reported at, when the redirect and the command
// it belongs to are not on the same line.
//
// Three answers among four shells, measured on a loop, a brace group and a
// command split by a backslash. bash names the redirect's own line, dash and
// zsh name the line the command began on, and ksh93 names the line before the
// redirect's in every shape — recorded as what it does rather than as what it
// might mean.
//
// Found on an installed script: /opt/homebrew/bin/missing_codec_desc opens a
// `while read` on line 5 and redirects it on line 11, and we said 5 where
// bash says 11.
func TestWhichLineAFailedOpenIsReportedAt(t *testing.T) {
	// The redirect is on line 4 and the command begins on line 2.
	const src = "echo one\nwhile read -r x\ndo\n  echo $x\ndone < /nonexistent/x\n"
	for _, tc := range []struct {
		name string
		at   RedirectLine
		want string
	}{
		{"the command's line", LineOfCommand, ":2:"},
		{"the redirect's line", LineOfRedirect, ":5:"},
		{"the line before it", LineBeforeRedirect, ":4:"},
		// A compound command, so this steps back like LineBeforeRedirect.
		{"the line before it, for a compound", LineBeforeRedirectWhenCompound, ":4:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var errs strings.Builder
			sem := PosixSemantics()
			dg := Diagnostics{
				RedirectFailureLine: tc.at,
				// A location style that shows the line, since the
				// substrate's own shows none.
				Location: LocationTightLine,
			}
			r := &Runner{
				Semantics: &sem, Diagnostics: &dg, Name: "sh",
				Stdout: &strings.Builder{}, Stderr: &errs,
			}
			f, err := syntax.Parse(src, syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(errs.String(), tc.want) {
				t.Errorf("said %q, want the line %s", errs.String(), tc.want)
			}
		})
	}
}

// The command is still reported where it was written: only the opening moves.
func TestOnlyTheOpeningTakesTheRedirectsLine(t *testing.T) {
	var errs strings.Builder
	sem := PosixSemantics()
	dg := Diagnostics{RedirectFailureLine: LineOfRedirect, Location: LocationTightLine}
	r := &Runner{
		Semantics: &sem, Diagnostics: &dg, Name: "sh",
		Stdout: &strings.Builder{}, Stderr: &errs,
	}
	// The failed open is on line 2; the command that is not found is on 3.
	f, err := syntax.Parse("echo one\ncat < /nonexistent/x\nnosuchcommand\n", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	got := errs.String()
	if !strings.Contains(got, ":2:") || !strings.Contains(got, ":3:") {
		t.Errorf("said %q, want the open at 2 and the command at 3", got)
	}
}

// redirLine runs src and returns what it wrote to standard error, with the
// line shown so a test can assert on it.
func redirLine(t *testing.T, at RedirectLine, src string) string {
	t.Helper()
	var errs strings.Builder
	sem := PosixSemantics()
	dg := Diagnostics{RedirectFailureLine: at, Location: LocationTightLine}
	r := &Runner{
		Semantics: &sem, Diagnostics: &dg, Name: "sh",
		Stdout: &strings.Builder{}, Stderr: &errs,
	}
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return errs.String()
}

// One answer treats a simple command differently from a compound one: the
// simple command is reported where it began and the compound one steps back a
// line, whatever the layout.
//
// The loop shape above cannot tell that answer from LineBeforeRedirect,
// because with the redirect already on a later line the two agree. These are
// the shapes that separate them, and they are why the axis has a fourth value
// rather than three.
func TestASimpleCommandMayBeReportedWhereItBegan(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		// Command and redirect on the same line: stepping back would name
		// line 1, and this must not.
		{"a simple command on one line", "echo one\ncat < /nonexistent/x\n", ":2:"},
		// The command word is on line 2 and the redirect on line 3, joined
		// by a backslash.
		{"a simple command split across lines", "echo one\ncat \\\n< /nonexistent/x\n", ":2:"},
		// A compound command on one line *does* step back, to line 1.
		{"a compound command on one line", "echo one\n{ echo hi; } < /nonexistent/x\n", ":1:"},
		{"a compound command across lines", "echo one\n{\n  echo hi\n} < /nonexistent/x\n", ":3:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := redirLine(t, LineBeforeRedirectWhenCompound, tc.src); !strings.Contains(got, tc.want) {
				t.Errorf("said %q, want the line %s", got, tc.want)
			}
		})
	}
	// And the plain "line before" answer steps back for a simple command too,
	// which is the whole difference between the two values.
	if got := redirLine(t, LineBeforeRedirect, "echo one\ncat < /nonexistent/x\n"); !strings.Contains(got, ":1:") {
		t.Errorf("said %q, want LineBeforeRedirect to step back for a simple command too", got)
	}
}

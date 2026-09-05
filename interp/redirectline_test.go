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

// Which line a failed open is reported at, for a *compound* command whose
// redirect is not on the line it opened on.
//
// Three answers among four shells: bash names the redirect's own line, dash
// and zsh name the line the command began on, and ksh93 names the line before
// the redirect's — recorded as what they do rather than as what they might
// mean. A simple command is the command's line in all four, which is why the
// axis speaks only about this case; see below.
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
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &dg, Name: "sh",
		Stdout: &strings.Builder{}, Stderr: &errs,
	})
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

// A simple command is reported where it began, whatever this axis says.
//
// All four dialects agree on that, which is why the axis only ever answers
// about a compound command. It took a command split by backslash
// continuations to see: with `cat` on line 2 and its redirect two physical
// lines below, every shell names line 2, and reading the redirect's own
// position gave bash line 4.
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
			if got := redirLine(t, LineBeforeRedirect, tc.src); !strings.Contains(got, tc.want) {
				t.Errorf("said %q, want the line %s", got, tc.want)
			}
		})
	}
	// And the other compound answer leaves a simple command alone too: every
	// dialect reports a simple command where it began, whatever this axis
	// says, which is why the axis only ever speaks about a compound one.
	if got := redirLine(t, LineOfRedirect, "echo one\ncat \\\n< /nonexistent/x\n"); !strings.Contains(got, ":2:") {
		t.Errorf("said %q, want a simple command reported where it began", got)
	}
}

// The moved line belongs to the *opening* and not to what the command then
// does. A compound command whose redirect succeeds still reports its body
// where the body is written.
//
// The existing check uses a separate statement afterwards, which sets the line
// itself and so cannot see a missing restore. This one has the diagnostic come
// from *inside* the command whose redirect moved the line.
func TestTheMovedLineDoesNotOutlastTheOpening(t *testing.T) {
	// The redirect is on line 4 and succeeds; the failing command is on 3.
	got := redirLine(t, LineOfRedirect, "echo one\n{\n  nosuchcommand\n} > /dev/null\n")
	if !strings.Contains(got, ":3:") {
		t.Errorf("said %q, want the body reported on its own line 3", got)
	}
	if strings.Contains(got, ":4:") {
		t.Errorf("said %q, want the redirect's line not to outlive the opening", got)
	}
}

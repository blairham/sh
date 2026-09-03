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

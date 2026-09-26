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

// [LineBeforeRedirect] counts one line below the redirect's, and **nought is
// a number it reaches**: on the first line of a file there is no line below,
// and the dialect that counts this way writes no line at all rather than the
// first one.
//
// Clamping the count to 1 there wrote `line 1` — a line of the file, and the
// wrong one — for every compound command written on line 1 of a script.
// Measured 2026-09-26 over a script file, `env -i PATH=/usr/bin:/bin
// HOME=<scratch> LC_ALL=C /bin/ksh f.sh` with stdin from /dev/null, against
// ksh93u+ 2012-08-01 (AT&T's own build, `not a Go executable` by `go version
// -m`):
//
//	{ echo RAN; } <<END with $(( 1/0 )) in it, on line 1   f.sh:  1/0 : divide by zero
//	the same on line 2                                     f.sh: line 1:  1/0 : …
//	( cat ) < nosuch on line 1                             f.sh: nosuch: cannot open …
//	the same on line 2                                     f.sh: line 1: nosuch: …
//	: <<END with the same body, on line 1                  f.sh[1]:  1/0 : divide by zero
//	the same on line 2                                     f.sh[2]:  1/0 : divide by zero
//
// The last pair is the control that says this is the `line N:` route alone:
// a simple command takes the bracketed form, which names 1 on line 1 and is
// not this axis's to move. See Runner.locatedWithoutALine (#4710).

// belowTheFirstLine runs src under an axis and returns what it wrote to
// standard error, with a location style that names every line — so a line
// that is *not* written is visible as an absence rather than as a style.
func belowTheFirstLine(t *testing.T, at RedirectLine, src string) string {
	t.Helper()
	var errs strings.Builder
	sem := PosixSemantics()
	dg := Diagnostics{RedirectFailureLine: at, Location: LocationLineWord}
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

func TestACountBelowTheFirstLineIsWrittenAsNoLineAtAll(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, src string
	}{
		{"a group", "{ echo hi; } < /nonexistent/x\n"},
		{"a subshell", "( echo hi ) < /nonexistent/x\n"},
		{"a loop", "while read -r x; do :; done < /nonexistent/x\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := belowTheFirstLine(t, LineBeforeRedirect, tc.src)
			if !strings.HasPrefix(got, "sh: cannot open /nonexistent/x") {
				t.Errorf("said %q, want the name alone in front of the reason", got)
			}
			if strings.Contains(got, "line ") {
				t.Errorf("said %q, want no line at all — there is none below the first", got)
			}
		})
	}
}

// The line below is still written whenever there is one, which is what says
// the rule above is about the count landing at nought and not about the first
// line of a file.
func TestTheLineBelowIsStillWrittenWhereThereIsOne(t *testing.T) {
	t.Parallel()
	got := belowTheFirstLine(t, LineBeforeRedirect, "echo one\n{ echo hi; } < /nonexistent/x\n")
	if !strings.Contains(got, "sh: line 1: cannot open /nonexistent/x") {
		t.Errorf("said %q, want the line below the redirect's", got)
	}
}

// And the two other answers are unmoved on the same line 1: they never count
// below it, so the absence above belongs to this axis alone.
func TestTheOtherRedirectLinesStillNameTheFirstLine(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		at   RedirectLine
	}{
		{"the command's line", LineOfCommand},
		{"the redirect's line", LineOfRedirect},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := belowTheFirstLine(t, tc.at, "{ echo hi; } < /nonexistent/x\n")
			if !strings.Contains(got, "sh: line 1: cannot open /nonexistent/x") {
				t.Errorf("said %q, want line 1 named", got)
			}
		})
	}
	// A *simple* command on line 1 is not this axis's to move either: every
	// dialect reports one where it began, so the count never steps back and
	// the line is written.
	got := belowTheFirstLine(t, LineBeforeRedirect, "cat < /nonexistent/x\n")
	if !strings.Contains(got, "sh: line 1: cannot open /nonexistent/x") {
		t.Errorf("said %q, want a simple command to keep its own line", got)
	}
}

// The absence does not outlast the opening: a command reported after one is
// located as it always was.
func TestTheMissingLineDoesNotOutlastTheOpening(t *testing.T) {
	t.Parallel()
	got := belowTheFirstLine(t, LineBeforeRedirect, "{ echo hi; } < /nonexistent/x\nnosuchcommand\n")
	if !strings.Contains(got, "sh: line 2: ") {
		t.Errorf("said %q, want the command on line 2 to carry its line", got)
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A here-document body is a program the shell reads on its own, so the line
// the second message quotes is a line of **that** program and not of the
// script the redirection is written in.
//
// The two are the same lines while the here-document is written in the
// script, which is why indexing the script worked for as long as it did. They
// part as soon as the body is inside something else the shell read: in a
// `$( … )` the body's lines are lines of that substitution's body, the index
// lands past the end of the file, and the message is not written at all.
//
// Measured 2026-09-26 over a script file, `env -i PATH=/usr/bin:/bin
// HOME=<scratch> LC_ALL=C /opt/homebrew/bin/bash d1.sh` with stdin from
// /dev/null, bash 5.3.20 (`not a Go executable` by `go version -m`), lines 1
// to 4 holding `v=$(cat <<END`, `$(echo hi; for)`, `END` and `)`:
//
//	bash 5.3.20  d1.sh: command substitution: line 5: syntax error near `)'
//	             d1.sh: command substitution: line 5: `echo hi; for)'
//	before       the first of those and nothing else
//
// The same body on a command of its own writes both lines in both shells,
// which is the control that says the second is produced in general and the
// `$( … )` is where it went missing. Counted as whole lines rather than with
// Contains: 2 against 1. See Runner.substFailureText (#4713).

// heredocEcho runs src as a whole program — the runner is told its text, as a
// front end tells it — and returns what it wrote to standard error.
func heredocEcho(t *testing.T, src string) string {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	var errs bytes.Buffer
	sem := PosixSemantics()
	// The reading under which a body that will not parse costs the command
	// and leaves the script running, so a row after it is still reached.
	sem.SubstitutionParseErrorIsFatal = Yes
	sem.SubstitutionParseFailureInAHeredocBodyEndsTheShell = No
	dg := Diagnostics{
		Location:               LocationLineWord,
		EchoesTheOffendingLine: true,
		SubstitutionParseFailureNamesTheConstruct: true,
		// The two that put a `$( … )` body's lines where the shell's reader
		// had got to, which is what carries a body inside one past the end
		// of the file — and so what makes the row this suite is about
		// reachable at all.
		SubstitutionBodyIsNumberedFromWhereTheShellWasReading: true,
		CommandIsLocatedWhereItsFirstWordEnds:                 true,
	}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &dg, Name: "sh",
		Stdout: &bytes.Buffer{}, Stderr: &errs,
	})
	r.SetProgramText(src)
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return errs.String()
}

func TestARefusedBodyIsQuotedAgainstTheBodysOwnText(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, want string }{
		{
			"a command of its own",
			"cat <<END\n$(echo hi; for)\nEND\n",
			"`echo hi; for)'",
		},
		{
			// The row the fix is about: the here-document is inside a
			// `$( … )`, so the body's line is not a line of the file.
			"inside a command substitution",
			"v=$(cat <<END\n$(echo hi; for)\nEND\n)\n",
			"`echo hi; for)'",
		},
		{
			"the body's second line",
			"cat <<END\nx\n$(echo hi; for)\nEND\n",
			"`echo hi; for)'",
		},
		{
			// The quote runs to the end of the body's line, which is what
			// says it is the line and not the substitution's own text.
			"text after the substitution",
			"cat <<END\nbefore $(echo hi; for) after\nEND\n",
			"`echo hi; for) after'",
		},
		{
			"a substitution across two body lines",
			"cat <<END\npad\n$(echo hi\nfor)\nEND\n",
			"`for)'",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := heredocEcho(t, tc.src)
			if !strings.Contains(got, tc.want) {
				t.Errorf("said %q, want %q in it", got, tc.want)
			}
			if n := strings.Count(got, "\n"); n != 2 {
				t.Errorf("said %q over %d lines, want the complaint and the quote", got, n)
			}
		})
	}
}

// The older spelling's body is read with the script's line, so it is quoted
// against the script — the control that says the rule above is keyed on the
// here-document and not on "text read at expansion time" at large.
func TestABackquotedBodyIsStillQuotedAgainstTheScript(t *testing.T) {
	t.Parallel()
	got := heredocEcho(t, "v=`echo $(for)`\n")
	if !strings.Contains(got, "`echo $(for)'") {
		t.Errorf("said %q, want the script's own line quoted", got)
	}
}

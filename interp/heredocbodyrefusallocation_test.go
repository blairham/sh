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

// A substitution refused in a **here-document body** stands at the line after
// the document in one column, and carries a plain sentence under it rather
// than the quote or the open-context line every other position gets.
//
// See interp/heredocbodyrefusallocation.go for the rows this is read off and
// for the one with no final newline, which is what says the `+1` is the
// reader's newline and not a constant (#4714).

// afterTheDelimiter runs src and returns what it wrote to standard error,
// with a location style that shows the line.
func afterTheDelimiter(t *testing.T, src string, after bool) string {
	t.Helper()
	var errs strings.Builder
	sem := PosixSemantics()
	sem.SubstitutionParseErrorIsFatal = No
	sem.SubstitutionParseFailureInAHeredocBodyEndsTheShell = No
	dg := Diagnostics{
		Location:         LocationTightLine,
		SyntaxUnexpected: "parse error near `%[1]s'",
		UnmatchedQuote:   `unmatched %[1]s`,
		HeredocBodyRefusalIsLocatedAfterTheDelimiter: after,
	}
	if after {
		dg.HeredocBodyRefusalSentence = "parse error"
	}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &dg, Name: "sh",
		Stdout: &strings.Builder{}, Stderr: &errs,
	})
	r.SetProgramText(src)
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return errs.String()
}

// Both messages stand at the delimiter's line, plus one where a line follows
// it — and they stand at the same line as each other.
func TestARefusedBodyStandsAfterTheDelimiter(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, want string }{
		{"a command of its own", "cat <<END\n$(echo hi; for)\nEND\n", "sh:4:"},
		{"with a line under it", "cat <<END\n$(echo hi; for)\nEND\necho after\n", "sh:4:"},
		{"padded above", "echo p\ncat <<END\n$(echo hi; for)\nEND\n", "sh:5:"},
		{"a two-line body", "cat <<END\nx\n$(echo hi; for)\nEND\n", "sh:5:"},
		{"a pipeline element", "cat <<END | cat\n$(echo hi; for)\nEND\n", "sh:4:"},
		{"a group", "{ cat; } <<END\n$(echo hi; for)\nEND\necho a\n", "sh:4:"},
		// The row that says the `+1` is the reader's newline: with the
		// delimiter as the last line of the file the number stops there.
		{"no newline after the delimiter", "cat <<END\n$(echo hi; for)\nEND", "sh:3:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := afterTheDelimiter(t, tc.src, true)
			if n := strings.Count(got, tc.want); n != 2 {
				t.Errorf("said %q with %d lines at %s, want both messages there", got, n, tc.want)
			}
			if !strings.Contains(got, tc.want+" parse error\n") {
				t.Errorf("said %q, want the plain sentence under the refusal", got)
			}
			if strings.Contains(got, "unmatched") {
				t.Errorf("said %q, want no open quote — the body holds none", got)
			}
			// And without the answer the messages are back in the body,
			// which is a different line in every row above.
			if off := afterTheDelimiter(t, tc.src, false); strings.Count(off, tc.want) == 2 {
				t.Errorf("with the answer off: said %q, want the body's own lines", off)
			}
		})
	}
}

// Everything that is not a here-document body is unmoved, which is what says
// the rule is keyed on the construct and not on the refusal.
func TestARefusalOutsideAHeredocBodyIsUnmoved(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src string }{
		{"an ordinary word", "echo $(echo hi; for)\n"},
		{"a quoted word", "echo \"$(echo hi; for)\"\n"},
		{"a here-string", "cat <<<\"$(echo hi; for)\"\n"},
		{"the older spelling", "v=`echo hi\nfor`\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			on, off := afterTheDelimiter(t, tc.src, true), afterTheDelimiter(t, tc.src, false)
			if on != off {
				t.Errorf("with the answer on %q and off %q, want the two the same", on, off)
			}
		})
	}
	// And a **quoted** delimiter is the control the body rows need: nothing
	// in it is expanded, so there is no refusal to place.
	if got := afterTheDelimiter(t, ": <<'END'\n$(echo hi; for)\nEND\n", true); got != "" {
		t.Errorf("a quoted delimiter said %q, want nothing refused", got)
	}
}

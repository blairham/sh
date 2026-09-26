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

// One column writes a line **inside** the sentence as well as in the
// location, and for a substitution refused in a here-document body the two
// are the same number: the line the message is located at, not the line the
// body holds the substitution on.
//
// See interp/heredocbodyrefusalline.go for the ten rows this is read off and
// for the two rules they carry (#4715).

// refusalSentenceLine runs src and returns what it wrote to standard error,
// with a wording that carries the line the way the dialect that has this does.
func refusalSentenceLine(t *testing.T, src string, namesTheLocation bool) string {
	t.Helper()
	var errs strings.Builder
	sem := PosixSemantics()
	sem.SubstitutionParseErrorIsFatal = No
	sem.SubstitutionParseFailureInAHeredocBodyEndsTheShell = No
	dg := Diagnostics{
		Location:                    LocationLineWord,
		ParseFailureNamesItsOwnLine: true,
		SyntaxUnexpected:            "at line %[3]d: `%[1]s' unexpected",
		// A compound command's failed redirection is reported a line below
		// the redirect's in the column that has this, which is what makes
		// the group rows a second position rather than a second spelling.
		RedirectFailureLine:                         LineBeforeRedirect,
		HeredocBodyRefusalNamesTheLineItIsLocatedAt: namesTheLocation,
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
	return errs.String()
}

// The sentence names the line the message is located at, wherever the command
// is written and whatever moved that location.
func TestARefusedBodysSentenceNamesTheLineItIsLocatedAt(t *testing.T) {
	t.Parallel()
	pad := func(n int) string { return strings.Repeat("echo p\n", n-1) }
	for _, tc := range []struct {
		name, command string
		at            int
		want          string
	}{
		{"a command of its own on line 1", "cat", 1, "at line 1:"},
		{"the same on line 2", "cat", 2, "at line 2:"},
		{"the same on line 3", "cat", 3, "at line 3:"},
		// A group's location is a line below the redirect's in this column,
		// and the sentence follows it there — including to nought.
		{"a group on line 1", "{ cat; }", 1, "at line 0:"},
		{"a group on line 2", "{ cat; }", 2, "at line 1:"},
		{"a group on line 3", "{ cat; }", 3, "at line 2:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := pad(tc.at) + tc.command + " <<END\n$(echo hi; for)\nEND\n"
			got := refusalSentenceLine(t, src, true)
			if !strings.Contains(got, tc.want) {
				t.Errorf("said %q, want %q in it", got, tc.want)
			}
			// And without the answer the sentence counts from the body,
			// which is a different number in every row above.
			if off := refusalSentenceLine(t, src, false); strings.Contains(off, tc.want) {
				t.Errorf("with the answer off: said %q, want a different line from %q", off, tc.want)
			}
		})
	}
}

// A pipeline element is the position the issue was measured on, and it is the
// command's own line there too.
func TestARefusedBodyOnAPipelineElementNamesTheCommandsLine(t *testing.T) {
	t.Parallel()
	got := refusalSentenceLine(t, "cat <<END | cat\n$(echo hi; for)\nEND\n", true)
	if !strings.Contains(got, "at line 1:") {
		t.Errorf("said %q, want `at line 1:`", got)
	}
	if off := refusalSentenceLine(t, "cat <<END | cat\n$(echo hi; for)\nEND\n", false); strings.Contains(off, "at line 1:") {
		t.Errorf("with the answer off: said %q, want the body's line instead", off)
	}
}

// And nought where the location is a speaking builtin's bracket: that counter
// is not the one the sentence reads.
func TestARefusedBodysSentenceIsNoughtUnderABuiltinsBracket(t *testing.T) {
	t.Parallel()
	for at := 1; at <= 3; at++ {
		src := strings.Repeat("echo p\n", at-1) + ": <<END\n$(echo hi; for)\nEND\n"
		var errs strings.Builder
		sem := PosixSemantics()
		sem.SubstitutionParseErrorIsFatal = No
		sem.SubstitutionParseFailureInAHeredocBodyEndsTheShell = No
		dg := Diagnostics{
			Location:                    LocationLineWord,
			BuiltinLocation:             LocationBracketLine,
			NamesBuiltinInLocation:      false,
			ParseFailureNamesItsOwnLine: true,
			SyntaxUnexpected:            "at line %[3]d: `%[1]s' unexpected",
			HeredocBodyRefusalNamesTheLineItIsLocatedAt: true,
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
		if got := errs.String(); !strings.Contains(got, "at line 0:") {
			t.Errorf("on line %d: said %q, want `at line 0:`", at, got)
		}
	}
}

// The same refusal in an ordinary **word** is unmoved, because there the
// body's line and the command's are the same line — which is what says this
// is the here-document's question and not the sentence's at large.
func TestARefusalInAWordKeepsItsOwnLine(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		"echo $(echo hi; for)\n",
		"echo p\necho $(echo hi; for)\n",
		"echo p\nv=`echo hi\nfor`\n",
	} {
		on, off := refusalSentenceLine(t, src, true), refusalSentenceLine(t, src, false)
		if on != off {
			t.Errorf("%q: with the answer on %q and off %q, want the two the same", src, on, off)
		}
	}
}

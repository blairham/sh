// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// An assignment word carries a blank where the arrangement asks for one, and
// that blank stands in for the separator to whatever follows it.
//
// Measured 2026-09-29 on zsh 5.9.2 through `which`, one function per row.
// The rows are asserted in pairs — with the blank asked for and without —
// because the second half is what says the field is the only thing moving.
func TestAnAssignmentTakesTheBlankItIsAskedFor(t *testing.T) {
	for _, tc := range []struct{ src, plain, spaced string }{
		// The blank shows at the end of a command whose last word is an
		// assignment.
		{"url=x", "url=x", "url=x "},
		// One blank between two assignments and not two: the first's blank
		// is the separator, which is the row a rule written as "print a
		// blank after each assignment *and* separate the words" gets wrong.
		{"a=1 b=2", "a=1 b=2", "a=1 b=2 "},
		// And none at the end where a word follows, which is the row a rule
		// written as "a command holding an assignment ends in a blank" gets
		// wrong. Both wrong rules agree with every other row here.
		{"a=1 print x", "a=1 print x", "a=1 print x"},
		// Two blanks before a redirection, because that one is written
		// separately from the separator this stands in for.
		{"url=x > out", "url=x > out", "url=x  > out"},
		// No assignment, no blank — in a command with no words either.
		{"print y", "print y", "print y"},
		{"true", "true", "true"},
		// The control that keeps it to assignments the *grammar* read as
		// assignments: the same characters as an argument to a command that
		// takes none are just a word.
		{"print url=x", "print url=x", "print url=x"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			if got := syntax.PrintFileWith(f, syntax.Layout{}); got != tc.plain {
				t.Errorf("without the blank: printed %q, want %q", got, tc.plain)
			}
			got := syntax.PrintFileWith(f, syntax.Layout{BlankAfterAnAssignment: true})
			if got != tc.spaced {
				t.Errorf("with the blank asked for: printed %q, want %q", got, tc.spaced)
			}
		})
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The trace attribute — the `t` letter of a declaration. It is recorded on
// the name, written back by every listing form, selected on by a filtered
// listing, taken off again by the plus sign, and read by nothing else: a
// traced name expands, splits and assigns exactly as it would without the
// letter. Tests name the tables and never a shell; what the panel does is in
// the corpus.

// withTraceLetter gives the synthetic dialect the letter, which is the whole
// of having the attribute — the `t` case of the parse and the tables under it
// are the core's.
func withTraceLetter(s *Semantics) {
	s.DeclareOptions = "aAfiprtx"
}

func TestTheTraceLetterIsRecordedAndListedBack(t *testing.T) {
	t.Parallel()
	out, errs, st := declRun(t, "typeset -t v=1; typeset -p v", withTraceLetter, Diagnostics{})
	if st != 0 || errs != "" {
		t.Fatalf("status %d, stderr %q; want a silent 0", st, errs)
	}
	if want := "declare -t v=\"1\"\n"; out != want {
		t.Errorf("listing = %q, want %q", out, want)
	}
}

// The letter on its own over a name that already holds a value adds the
// attribute and leaves the value where it is, which is what says it is a
// property of the name rather than of the assignment.
func TestTheTraceLetterAloneKeepsTheValue(t *testing.T) {
	t.Parallel()
	out, _, st := declRun(t, "v=9; typeset -t v; typeset -p v", withTraceLetter, Diagnostics{})
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	if want := "declare -t v=\"9\"\n"; out != want {
		t.Errorf("listing = %q, want %q", out, want)
	}
}

// And the plus takes it off and leaves everything else standing.
func TestThePlusSignTakesTheTraceLetterOff(t *testing.T) {
	t.Parallel()
	out, _, st := declRun(t,
		"typeset -tx v=1; typeset +t v; typeset -p v", withTraceLetter, Diagnostics{})
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	if want := "declare -x v=\"1\"\n"; out != want {
		t.Errorf("listing = %q, want %q", out, want)
	}
}

// A traced name is an ordinary name everywhere else: the value reads back
// whole, and an assignment through it is not changed by the letter. This is
// the row that says the attribute is inert, which is the measured half the
// listing rows cannot state.
func TestATracedNameExpandsAndAssignsLikeAnyOther(t *testing.T) {
	t.Parallel()
	out, _, st := declRun(t,
		"typeset -t v=one; echo [$v]; v=two; echo [$v]", withTraceLetter, Diagnostics{})
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	if want := "[one]\n[two]\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// The filtered listing selects on it the way it selects on every other
// attribute letter, and the whole-table walk reaches a name whose only
// attribute is this one — the table is in none of the others.
func TestTheTraceLetterFiltersAListing(t *testing.T) {
	t.Parallel()
	out, _, st := declRun(t,
		"typeset -t a=1; b=2; typeset -t", withTraceLetter, Diagnostics{})
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	if want := "typeset a=\"1\"\n"; out != want {
		t.Errorf("filtered listing = %q, want %q", out, want)
	}
}

// A declaration inside a call is a fresh binding and gives the outer name's
// letters back on return, which is the rule every attribute in
// interp/localattributes.go follows and the reason this one travels with
// them rather than in a table of its own.
func TestACallsTraceLetterDoesNotOutliveTheCall(t *testing.T) {
	t.Parallel()
	set := func(s *Semantics) {
		withTraceLetter(s)
		s.LocalOptions = s.DeclareOptions
	}
	out, _, st := declRun(t,
		"v=1; f() { local -t v=2; typeset -p v; }; f; typeset -p v", set, Diagnostics{})
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	want := "declare -t v=\"2\"\ndeclare -- v=\"1\"\n"
	if out != want {
		t.Errorf("listings = %q, want %q", out, want)
	}
}

// And the letter a dialect does not spell is still refused, which is what
// says the parse reads it from DeclareOptions rather than from the switch it
// happens to have a case in.
func TestADialectWithoutTheTraceLetterRefusesIt(t *testing.T) {
	t.Parallel()
	_, errs, st := declRun(t, "typeset -t v=1", func(s *Semantics) {
		s.DeclareOptions = "aAfiprx"
	}, Diagnostics{})
	if st == 0 {
		t.Fatalf("status 0 for a letter the dialect has not got; want a refusal")
	}
	if !strings.Contains(errs, "-t") {
		t.Errorf("stderr = %q, want the letter named", errs)
	}
}

// The function half of the same letter, which is a mark on the function
// rather than an attribute of a variable: it is recorded, a whole-table
// listing writes the row under the body, and a listing carrying the letter
// narrows to the functions holding it. See Semantics.FunctionAttributeLetters.
func TestTheTraceLetterMarksAFunctionAndNarrowsTheListing(t *testing.T) {
	t.Parallel()
	set := func(s *Semantics) {
		withTraceLetter(s)
		s.DeclareOptions = "aAfFiprtx"
		s.FunctionAttributeLetters = "rtx"
	}
	out, errs, st := declRun(t,
		"a() { :; }; b() { :; }; typeset -ft a; typeset -Ft", set, Diagnostics{})
	if st != 0 || errs != "" {
		t.Fatalf("status %d, stderr %q; want a silent 0", st, errs)
	}
	if want := "declare -ft a\n"; out != want {
		t.Errorf("narrowed listing = %q, want %q", out, want)
	}
}

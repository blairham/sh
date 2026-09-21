// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Whose names `local -p name` writes — the running call's own, or every name
// the call can see. See Semantics.LocalListingIsTheRunningCallsOwn, which
// carries the measurement.
//
// The row itself is the declaration builtin's under either answer, so what
// these pin is the *set*: a name the running call declared lists identically
// both ways, and the two part company only over one it did not.

// localListingScope is declRun's setter for the axis, with the letters the
// form needs read on the word: `-p` has to reach `local` at all before whose
// names it writes can be a question.
func localListingScope(a Answer) func(*Semantics) {
	return func(s *Semantics) {
		s.LocalListingIsTheRunningCallsOwn = a
		s.LocalOptions = "agiprux"
		// The sentence a name that is not there gets, which is the half of
		// the refusal this axis borrows rather than writes again. Answered
		// here so a suite about *whose* names are listed is not stopped by a
		// second question about whether an absent one is reported at all.
		s.DeclarePrintReportsAMissingName = Yes
	}
}

// TestLocalPrintNamesOnlyTheRunningCallsLocals is the answer that reads the
// word as the call's: a global and a *caller's* local are both absent, and
// the sentence is the one a missing operand already gets with this builtin's
// name in front of it.
func TestLocalPrintNamesOnlyTheRunningCallsLocals(t *testing.T) {
	const src = `g=global
inner() { local -p g; echo "st=$?"; }
outer() { local g=caller; inner; }
outer`
	out, errs, st := declRun(t, src, localListingScope(Yes), Diagnostics{})
	if want := "st=1\n"; out != want {
		t.Errorf("stdout = %q, want %q — a name this call did not declare must not list", out, want)
	}
	if !strings.Contains(errs, "local: g: not found") {
		t.Errorf("stderr = %q, want the builtin's own missing-name sentence", errs)
	}
	if st != 0 {
		t.Errorf("status %d, want 0 — the function ran on past the refusal", st)
	}
}

// And the call's own local still lists, which is the control that keeps the
// answer above from being "local -p writes nothing".
func TestLocalPrintNamesTheRunningCallsOwnLocal(t *testing.T) {
	const src = `g=global
f() { local g=mine; local -p g; echo "st=$?"; }
f`
	out, errs, st := declRun(t, src, localListingScope(Yes), Diagnostics{})
	want := "declare -- g=\"mine\"\nst=0\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("a call's own local = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// TestLocalPrintListsANameThisCallDidNotDeclare is the other answer: the word
// narrows nothing, so the name is found wherever it lives and is written.
func TestLocalPrintListsANameThisCallDidNotDeclare(t *testing.T) {
	const src = `g=global
inner() { local -p g; echo "st=$?"; }
outer() { local g=caller; inner; }
outer`
	out, errs, st := declRun(t, src, localListingScope(No), Diagnostics{})
	want := "declare -- g=\"caller\"\nst=0\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("an unnarrowed listing = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The narrowing is this word's and not the listing's: `declare -p` over the
// same name is unmoved under either answer, which is what says the axis is
// about which builtin asked rather than about the row.
func TestTheDeclarationWordsListingIsNotNarrowed(t *testing.T) {
	const src = `g=global
inner() { typeset -p g; echo "st=$?"; }
outer() { local g=caller; inner; }
outer`
	for _, a := range []Answer{Yes, No} {
		out, errs, st := declRun(t, src, localListingScope(a), Diagnostics{})
		want := "declare -- g=\"caller\"\nst=0\n"
		if out != want || errs != "" || st != 0 {
			t.Errorf("`typeset -p` under %v = %q (stderr %q, status %d), want %q", a, out, errs, st, want)
		}
	}
}

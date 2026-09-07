// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The letters `local` reads are the dialect's — LocalOptions — and may be
// none at all, in which case a `-` word is an operand: a name, and a bad one.
// What a bare `local` writes is BareLocalListingForm's to answer.

// TestLocalReadsTheDialectsLetters: with letters, `local -i` declares the
// attribute and `local -r` freezes the local; the caller's values come back
// either way.
func TestLocalReadsTheDialectsLetters(t *testing.T) {
	src := "n=keep\nf() { local -i n; n=2+3; echo in=$n; }\nf\necho out=$n"
	out, errs, _ := declRun(t, src, func(s *Semantics) { s.LocalOptions = "air" }, Diagnostics{})
	if errs != "" {
		t.Fatalf("stderr = %q, want none", errs)
	}
	if !strings.Contains(out, "in=5") {
		t.Errorf("stdout = %q, want the integer attribute evaluating the assignment", out)
	}
	if !strings.Contains(out, "out=keep") {
		t.Errorf("stdout = %q, want the outer value back", out)
	}

	src = "f() { local -r ro=1; ro=2; echo unreached; }\nf\necho after"
	out, errs, _ = declRun(t, src, func(s *Semantics) {
		s.LocalOptions = "air"
		s.ReadonlyReassignmentFatal = Yes
	}, Diagnostics{})
	if !strings.Contains(errs, "readonly") {
		t.Errorf("stderr = %q, want the readonly refusal", errs)
	}
	if strings.Contains(out, "unreached") {
		t.Errorf("stdout = %q, want the reassignment refused", out)
	}
}

// TestWithoutLettersALocalOptionIsAName: LocalOptions empty means `local -r`
// declares a variable named `-r`, which the name rules then refuse — fatally
// where the dialect says a declaration's bad name is fatal.
func TestWithoutLettersALocalOptionIsAName(t *testing.T) {
	src := "f() { local -r x=5; echo unreached; }\nf\necho after"
	out, errs, st := declRun(t, src, func(s *Semantics) {
		s.LocalOptions = ""
		s.DeclarationNameOperands = PlainNamesOnly
		s.BadNameToDeclarationFatal = Yes
	}, Diagnostics{BuiltinBadName: map[string]string{"local": "%[1]s: %[2]s: bad variable name"}})
	if !strings.Contains(errs, "local: -r: bad variable name") {
		t.Errorf("stderr = %q, want the operand refused as a name", errs)
	}
	if strings.Contains(out, "unreached") || strings.Contains(out, "after") || st == 0 {
		t.Errorf("stdout %q status %d, want the bad name to end the script", out, st)
	}
}

// TestBareLocalLists covers the three answers: the innermost function's own
// locals as clustered declarations, nothing at all, and the refusal of the
// engine whose listing is its whole parameter table.
func TestBareLocalLists(t *testing.T) {
	src := "g() { local outer=1; f; }\nf() { local -i n=5; local x; local; }\ng"
	out, errs, st := declRun(t, src, func(s *Semantics) {
		s.LocalOptions = "air"
		s.BareLocalListing = BareLocalListsLocals
	}, Diagnostics{})
	if errs != "" || st != 0 {
		t.Fatalf("stderr %q status %d, want a clean listing", errs, st)
	}
	if want := "declare -i n=\"5\"\ndeclare -- x\n"; out != want {
		t.Errorf("stdout = %q, want %q — the innermost locals only, sorted", out, want)
	}

	out, errs, st = declRun(t, "f() { local x=1; local; }\nf", func(s *Semantics) {
		s.BareLocalListing = BareLocalListsNothing
	}, Diagnostics{})
	if out != "" || errs != "" || st != 0 {
		t.Errorf("out %q errs %q status %d, want silence and 0", out, errs, st)
	}

	// The third form is neither of the other two: every parameter the shell
	// holds, not the scope's alone, with the attributes written as words
	// before the assignment and `local` among them for a name the running
	// function made local.
	out, errs, st = declRun(t, "f() { local x=1; readonly -p >/dev/null; local; }\nf", func(s *Semantics) {
		s.BareLocalListing = BareLocalListsEveryParameter
	}, Diagnostics{})
	if errs != "" || st != 0 {
		t.Fatalf("errs %q status %d, want a listing and 0", errs, st)
	}
	if want := "local x=\"1\"\n"; !strings.Contains(out, want) {
		t.Errorf("out %q, want %q in it", out, want)
	}
	if !strings.Contains(out, "\nOPTIND=") {
		t.Errorf("out %q, want a global listed with no local word", out)
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `readonly -a` and `readonly -A` declaring the kind as well as freezing the
// name — see Semantics.ReadonlyRecordsTheCompoundAttribute and #1554.
//
// Named for the axis and never for a shell.

func readonlyCompoundRun(t *testing.T, src string, a Answer) (string, string, int) {
	t.Helper()
	return declRun(t, src, func(s *Semantics) {
		s.DeclareOptions = "aAgiprx"
		s.TypesetLocalNeedsKeywordFunction = No
		s.ReadonlyRecordsTheCompoundAttribute = a
	}, Diagnostics{})
}

// The headline, both ways: under Yes the listing carries the kind, under No
// the name is frozen and its kind is not recorded.
func TestReadonlyRecordingTheCompoundIsAnAxis(t *testing.T) {
	const src = `f() { readonly -a a; typeset -p a; }; f`
	out, errs, st := readonlyCompoundRun(t, src, Yes)
	// The kind and no value, which is the shape the keyed half below has
	// always had: a declaration that writes nothing lists without the `=()`.
	// This row wanted `declare -ar a=()` until #2558, so the pair disagreed
	// with itself about the same state.
	if want := "declare -ar a\n"; out != want || st != 0 || errs != "" {
		t.Errorf("yes: got %q/%d stderr %q, want %q", out, st, errs, want)
	}
	out, errs, st = readonlyCompoundRun(t, src, No)
	if want := "declare -r a\n"; out != want || st != 0 || errs != "" {
		t.Errorf("no: got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}

// The keyed half moves with it, which is what makes this one question.
func TestReadonlyRecordsTheTableToo(t *testing.T) {
	const src = `f() { readonly -A m; typeset -p m; }; f`
	out, _, _ := readonlyCompoundRun(t, src, Yes)
	if want := "declare -Ar m\n"; out != want {
		t.Errorf("yes: got %q, want %q", out, want)
	}
	out, _, _ = readonlyCompoundRun(t, src, No)
	if want := "declare -r m\n"; out != want {
		t.Errorf("no: got %q, want %q", out, want)
	}
}

// A `readonly` with no kind letter raises no question, so nothing is asked and
// an unanswered axis is not a refusal.
func TestReadonlyWithNoKindLetterAsksNothing(t *testing.T) {
	out, errs, st := declRun(t, `readonly r=1; typeset -p r`, func(s *Semantics) {
		s.DeclareOptions = "aAgiprx"
		s.ReadonlyRecordsTheCompoundAttribute = Unspecified
	}, Diagnostics{})
	if want := `declare -r r="1"` + "\n"; out != want || st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want %q", out, st, errs, want)
	}
	// And with the letter it is, which says the silence above is the guard.
	out, errs, _ = declRun(t, `readonly -a a`, func(s *Semantics) {
		s.DeclareOptions = "aAgiprx"
		s.ReadonlyRecordsTheCompoundAttribute = Unspecified
	}, Diagnostics{})
	if !strings.Contains(errs, "the array letter on `readonly` declaring an array") {
		t.Errorf("got %q stderr %q, want a refusal naming the axis", out, errs)
	}
}

// The value a `readonly -a` carries is not refused by the attribute it
// carries beside it, and the mark does not replace it either — the elements
// are the ones the command was given.
func TestReadonlyWithAnArrayValueKeepsTheElements(t *testing.T) {
	out, errs, st := readonlyCompoundRun(t, `readonly -a a=(p q); typeset -p a`, Yes)
	if want := `declare -ar a=([0]="p" [1]="q")` + "\n"; out != want || st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}

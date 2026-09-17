// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A declaration that names an attribute over a **frozen** name — see
// Semantics.AttributeOverAFrozenNameIsRefused and #2561 — and the two things
// that keep the refusal from being wider than it was measured: the letters it
// does not cover, and the retype one dialect exempts.
//
// Tests name axes and wordings, never shells.

func frozenAttrRun(t *testing.T, src string, a Answer) (string, string, int) {
	t.Helper()
	return declRun(t, src, func(s *Semantics) {
		s.DeclareOptions = "aAFgiprxul"
		s.DeclareOptionsTakingANumber = "Fi"
		s.TypesetLocalNeedsKeywordFunction = No
		// The valueless row needs the standing text re-read, or there is no
		// listing to read the letter back out of.
		s.AttributeRereadsTheValueItFinds = Yes
		// The exemption is asked first, so a suite about this axis has to
		// turn it off or nothing reaches the question.
		s.NumericTypeLetterRetypesAFrozenName = No
		// The refusal has to leave the script running, or there is no
		// listing after it to read the attributes back out of — which is the
		// half this axis is about. The suite that asks whether a refused
		// declaration is fatal is readonlyfatal's, not this one.
		s.ReadonlyReassignmentByDeclarationFatal = No
		s.ReadonlyReassignmentBySpecialBuiltinFatal = No
		s.AttributeOverAFrozenNameIsRefused = a
	}, Diagnostics{ReadonlyVariable: "%s: readonly variable"})
}

// The headline, on the form that has no value at all: under Yes the operand is
// refused and the name keeps exactly the attributes it had, under No the
// letter lands and the declaration is silent.
func TestAnAttributeOverAFrozenNameIsAnAxis(t *testing.T) {
	src := `readonly q=1; typeset -gi q; echo st=$?; typeset -p q`
	out, errs, _ := frozenAttrRun(t, src, No)
	if !strings.Contains(out, "st=0") || errs != "" {
		t.Errorf("no: got %q stderr %q, want the letter taken in silence", out, errs)
	}
	if !strings.Contains(out, "-i") {
		t.Errorf("no: got %q, want the integer letter recorded", out)
	}
	out, errs, _ = frozenAttrRun(t, src, Yes)
	if !strings.Contains(errs, "q: readonly variable") || !strings.Contains(out, "st=1") {
		t.Errorf("yes: got %q stderr %q, want the refusal at 1", out, errs)
	}
	if strings.Contains(out, "-i") {
		t.Errorf("yes: got %q, want no integer letter on a refused declaration", out)
	}
}

// The half the refusal is *for*, and the reason it stands in front of the
// attributes rather than inside the store: a declaration that also assigns was
// already refused at the store, and the letter it named was recorded on the
// way past — so the name came out carrying an attribute the declaration never
// managed to make. Under Yes nothing of the operand happens.
func TestARefusedDeclarationOverAFrozenNameRecordsNoAttribute(t *testing.T) {
	src := `readonly q=1; typeset -gi q=4; typeset -p q; echo tail`
	out, errs, _ := frozenAttrRun(t, src, Yes)
	if !strings.Contains(errs, "q: readonly variable") {
		t.Fatalf("got stderr %q, want the refusal", errs)
	}
	if strings.Contains(out, "-i") || !strings.Contains(out, `q="1"`) {
		t.Errorf("got %q, want the name listed with neither the letter nor the value", out)
	}
}

// The sign is not read. A letter written under a plus is a change to the
// name's attributes like any other, so the refusal covers the removal as well
// as the addition — a guard that asked whether a type was being *added* would
// let this through.
func TestTheRefusalCoversALetterUnderAPlus(t *testing.T) {
	src := `typeset -gir q=1; typeset -g +i q; typeset -p q; echo tail`
	out, errs, _ := frozenAttrRun(t, src, Yes)
	if !strings.Contains(errs, "q: readonly variable") {
		t.Errorf("got %q stderr %q, want the refusal", out, errs)
	}
	if !strings.Contains(out, "-i") {
		t.Errorf("got %q, want the letter the name already had still on it", out)
	}
}

// And the letters it does not cover, which is what keeps it from being "a
// frozen name refuses every declaration": a permission and a placement are not
// what a value looks like, and all four columns take them.
func TestAPermissionLetterOverAFrozenNameIsStillTaken(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"export", `readonly q=1; typeset -gx q; typeset -p q`, "-rx"},
		{"readonly", `readonly q=1; typeset -gr q; typeset -p q`, "-r"},
		{"no letter", `readonly q=1; typeset -g q; typeset -p q`, `q="1"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := frozenAttrRun(t, tc.src, Yes)
			if errs != "" || st != 0 {
				t.Errorf("got %q/%d stderr %q, want it taken", out, st, errs)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want it to contain %q", out, tc.want)
			}
		})
	}
}

// The exemption is asked first, so a dialect that lets a numeric type letter
// retype a frozen name never reaches this refusal — the two fields would
// otherwise be a pair that can be set to contradict each other.
func TestARetypeExemptionOutranksTheAttributeRefusal(t *testing.T) {
	out, errs, st := declRun(t, `readonly q=1; typeset -gi q=4; typeset -p q; echo tail`,
		func(s *Semantics) {
			s.DeclareOptions = "aAFgiprxul"
			s.DeclareOptionsTakingANumber = "Fi"
			s.TypesetLocalNeedsKeywordFunction = No
			s.AttributeRereadsTheValueItFinds = Yes
			s.ReadonlyReassignmentByDeclarationFatal = No
			s.ReadonlyReassignmentBySpecialBuiltinFatal = No
			s.NumericTypeLetterRetypesAFrozenName = Yes
			s.AttributeOverAFrozenNameIsRefused = Yes
		}, Diagnostics{ReadonlyVariable: "%s: readonly variable"})
	if errs != "" || st != 0 || !strings.Contains(out, "tail") {
		t.Errorf("got %q/%d stderr %q, want the retype taken", out, st, errs)
	}
	if !strings.Contains(out, "-i") || strings.Contains(out, `q="1"`) {
		t.Errorf("got %q, want the letter and the new value", out)
	}
}

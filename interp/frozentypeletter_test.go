// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The complement of frozenattribute_test's axis: where a dialect takes an
// attribute letter over a frozen name in general, it may still refuse the
// letters a **value has to be built for** over a frozen name that holds
// nothing. See Semantics.TypeLetterOverAFrozenNameWithNoValueIsRefused and
// #3937. Tests name axes and wordings, never shells.
//
// Three controls carry the weight here, because the headline row alone is
// satisfied by a refusal that is far too wide:
//
//   - the **same letter with a value** is taken under both answers, which is
//     what says the rule is about what the name holds;
//   - the **case and array letters** are taken under both, which is what says
//     it is a narrower set than the wide axis's and not that axis reached
//     again;
//   - the **wide axis outranks it**, so a dialect that refuses every
//     attribute over a frozen name never reaches this question and cannot be
//     made to contradict itself.

// frozenTypeLetterRun runs src with the wide axis answered No — or there is
// nothing for the narrow one to be reached by — and the narrow one at a.
func frozenTypeLetterRun(t *testing.T, src string, a Answer) (string, string, int) {
	t.Helper()
	return declRun(t, src, func(s *Semantics) {
		s.DeclareOptions = "aACFgiprxultLRZ"
		s.DeclareOptionsTakingANumber = "FiLRZ"
		s.TypesetLocalNeedsKeywordFunction = No
		s.AttributeRereadsTheValueItFinds = Yes
		// Asked first and takes its own names out, so the suite has to turn
		// it off or nothing reaches the question this file is about.
		s.NumericTypeLetterRetypesAFrozenName = No
		// And the wide axis, for the same reason.
		s.AttributeOverAFrozenNameIsRefused = No
		// The refusal has to leave the script running, or there is no
		// listing after it to read the attributes back out of.
		s.ReadonlyReassignmentByDeclarationFatal = No
		s.ReadonlyReassignmentBySpecialBuiltinFatal = No
		s.TypeLetterOverAFrozenNameWithNoValueIsRefused = a
	}, Diagnostics{ReadonlyVariable: "%s: readonly variable"})
}

// The headline, on each letter in the set: under Yes the operand is refused
// and the name keeps the attributes it had, under No the letter lands.
func TestATypeLetterOverAFrozenNameWithNoValueIsAnAxis(t *testing.T) {
	// The listing is asserted only where this vector writes the letter back
	// for a name with no value; the other two rows are about the refusal,
	// and a listing row they cannot reach would be a test of the listing.
	for _, tc := range []struct{ name, decl, letter string }{
		{"integer", "typeset -gi q", "-i"},
		{"float", "typeset -gF q", ""},
		{"field width", "typeset -gL3 q", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `readonly q; ` + tc.decl + `; echo st=$?; typeset -p q`
			out, errs, _ := frozenTypeLetterRun(t, src, No)
			if !strings.Contains(out, "st=0") || errs != "" {
				t.Errorf("no: got %q stderr %q, want the letter taken in silence", out, errs)
			}
			if tc.letter != "" && !strings.Contains(out, tc.letter) {
				t.Errorf("no: got %q, want %q recorded", out, tc.letter)
			}
			out, errs, _ = frozenTypeLetterRun(t, src, Yes)
			if !strings.Contains(errs, "q: readonly variable") || !strings.Contains(out, "st=1") {
				t.Errorf("yes: got %q stderr %q, want the refusal at 1", out, errs)
			}
			if tc.letter != "" && strings.Contains(out, tc.letter) {
				t.Errorf("yes: got %q, want no %q on a refused declaration", out, tc.letter)
			}
		})
	}
}

// The control that says the rule is about the *value* and not about the
// letter: the same declaration over a frozen name that holds one is taken
// whichever way the axis is answered. A refusal keyed on the letter alone
// passes every row above and fails this one.
func TestTheSameTypeLetterOverAFrozenNameHoldingAValueIsTaken(t *testing.T) {
	const src = `q=1; readonly q; typeset -gi q; echo st=$?; typeset -p q`
	for _, a := range []Answer{Yes, No} {
		out, errs, _ := frozenTypeLetterRun(t, src, a)
		if !strings.Contains(out, "st=0") || errs != "" {
			t.Errorf("%v: got %q stderr %q, want the letter taken", a, out, errs)
		}
		if !strings.Contains(out, "-i") {
			t.Errorf("%v: got %q, want the integer letter recorded", a, out)
		}
	}
}

// An empty value is a value. `q=; readonly q` holds the empty string rather
// than nothing, and the axis does not reach it — the row that says "holds
// nothing" means unset.
func TestAnEmptyValueIsNotNoValueForTheTypeLetterRefusal(t *testing.T) {
	const src = `q=; readonly q; typeset -gi q; echo st=$?`
	out, errs, _ := frozenTypeLetterRun(t, src, Yes)
	if !strings.Contains(out, "st=0") || errs != "" {
		t.Errorf("got %q stderr %q, want the letter taken over a name holding the empty string", out, errs)
	}
}

// The letters outside the set, which is what keeps this from being the wide
// axis reached a second time: the two case letters and the two array letters
// are in *that* one and are taken here under both answers, and so are the
// permissions.
func TestTheLettersOutsideTheTypeSetAreTakenOverAValuelessFrozenName(t *testing.T) {
	for _, tc := range []struct{ name, decl string }{
		{"upper", "typeset -gu q"},
		{"lower", "typeset -gl q"},
		{"array", "typeset -ga q"},
		{"associative", "typeset -gA q"},
		{"export", "typeset -gx q"},
		{"trace", "typeset -gt q"},
		{"no letter", "typeset -g q"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `readonly q; ` + tc.decl + `; echo st=$?`
			for _, a := range []Answer{Yes, No} {
				out, errs, _ := frozenTypeLetterRun(t, src, a)
				if !strings.Contains(out, "st=0") || errs != "" {
					t.Errorf("%v: got %q stderr %q, want it taken", a, out, errs)
				}
			}
		})
	}
}

// The wide axis is asked first, so a dialect that refuses every value-shaping
// attribute over a frozen name never reaches the narrow question and the pair
// cannot be set to contradict each other.
func TestTheWideAttributeRefusalOutranksTheTypeLetterOne(t *testing.T) {
	out, errs, _ := declRun(t, `q=1; readonly q; typeset -gi q; echo st=$?`,
		func(s *Semantics) {
			s.DeclareOptions = "aAFgiprxul"
			s.DeclareOptionsTakingANumber = "Fi"
			s.TypesetLocalNeedsKeywordFunction = No
			s.AttributeRereadsTheValueItFinds = Yes
			s.NumericTypeLetterRetypesAFrozenName = No
			s.ReadonlyReassignmentByDeclarationFatal = No
			s.ReadonlyReassignmentBySpecialBuiltinFatal = No
			s.AttributeOverAFrozenNameIsRefused = Yes
			// The name holds a value, so the narrow axis would take this
			// line whatever it said. Answered No to make the point that the
			// refusal below comes from the wide one.
			s.TypeLetterOverAFrozenNameWithNoValueIsRefused = No
		}, Diagnostics{ReadonlyVariable: "%s: readonly variable"})
	if !strings.Contains(errs, "q: readonly variable") || !strings.Contains(out, "st=1") {
		t.Errorf("got %q stderr %q, want the wide axis's refusal", out, errs)
	}
}

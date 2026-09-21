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

// An operand that also **assigns** is not this axis: it is decided at the
// store, and this refusal must not reach it. The discriminating form of that
// is the wording, which this vector does not separate — dialect/ksh's
// frozentypeletter_test asks it where the two refusals are worded apart.
// What is checkable here is the *literal* spelling, since an array or
// compound literal reaches the declaration loop as a bare name and a guard
// reading only `name=value` would call it valueless.
func TestAnArrayLiteralOperandIsOutsideTheTypeLetterRefusal(t *testing.T) {
	out, errs, _ := frozenTypeLetterRun(t,
		`readonly q; typeset -gia q=(1 2); echo st=$?`, Yes)
	if strings.Contains(errs, "q: readonly variable") && strings.Contains(out, "st=1") {
		// Only this refusal is ruled out; whether the *store* refuses the
		// literal is the assignment's own rule and another suite's subject.
		// Told apart by the letter never landing, which is what this one
		// does and the store's refusal does not.
		out2, _, _ := frozenTypeLetterRun(t,
			`readonly q; typeset -gia q=(1 2); typeset -p q`, Yes)
		if !strings.Contains(out2, "-i") {
			t.Errorf("got %q, want the literal operand decided at the store rather "+
				"than refused for its letter", out2)
		}
	}
}

// The mirror image, and the reason the two are two fields: the letters whose
// cell cannot *hold* what a frozen name already has are refused where there
// is a value, which is exactly the column the axis above leaves alone. See
// Semantics.KeyedLetterOverAFrozenNameHoldingAValueIsRefused and #3965.
//
// One control carries this one: the **indexed** array letter is taken under
// both answers, on the same frozen name holding the same value. A refusal
// reading "a container letter" instead of "a keyed one" passes the headline
// and fails that row.

// frozenKeyedRun is frozenTypeLetterRun with the other narrow axis moved and
// this one's own answer held at No, so a failure names one field.
func frozenKeyedRun(t *testing.T, src string, a Answer) (string, string, int) {
	t.Helper()
	return declRun(t, src, func(s *Semantics) {
		s.DeclareOptions = "aACFgiprxultLRZ"
		s.DeclareOptionsTakingANumber = "FiLRZ"
		s.TypesetLocalNeedsKeywordFunction = No
		s.AttributeRereadsTheValueItFinds = Yes
		s.NumericTypeLetterRetypesAFrozenName = No
		s.AttributeOverAFrozenNameIsRefused = No
		s.TypeLetterOverAFrozenNameWithNoValueIsRefused = No
		s.ReadonlyReassignmentByDeclarationFatal = No
		s.ReadonlyReassignmentBySpecialBuiltinFatal = No
		// A container letter over a name already holding a scalar asks two
		// further questions of its own, and a run that leaves them
		// unanswered refuses before this one is reached. Answered the same
		// way under both legs, which is what keeps the rows comparable.
		s.ScalarUnderAnArrayDeclaration = ScalarUnderACompoundStaysAScalar
		s.ScalarUnderATableDeclaration = ScalarUnderACompoundStaysAScalar
		s.KeyedLetterOverAFrozenNameHoldingAValueIsRefused = a
	}, Diagnostics{ReadonlyVariable: "%s: readonly variable"})
}

func TestAKeyedLetterOverAFrozenNameHoldingAValueIsAnAxis(t *testing.T) {
	const src = `q=1; readonly q; typeset -gA q; echo st=$?`
	out, errs, _ := frozenKeyedRun(t, src, No)
	if !strings.Contains(out, "st=0") || errs != "" {
		t.Errorf("no: got %q stderr %q, want the letter taken in silence", out, errs)
	}
	out, errs, _ = frozenKeyedRun(t, src, Yes)
	if !strings.Contains(errs, "q: readonly variable") || !strings.Contains(out, "st=1") {
		t.Errorf("yes: got %q stderr %q, want the refusal at 1", out, errs)
	}
}

// The indexed array letter is outside the set, and an empty value is still a
// value. Both under either answer.
func TestTheIndexedArrayLetterIsOutsideTheKeyedRefusal(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"indexed over a value", `q=1; readonly q; typeset -ga q; echo st=$?`},
		{"indexed over an empty value", `q=; readonly q; typeset -ga q; echo st=$?`},
		{"integer over a value", `q=1; readonly q; typeset -gi q; echo st=$?`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, a := range []Answer{Yes, No} {
				out, errs, _ := frozenKeyedRun(t, tc.src, a)
				if !strings.Contains(out, "st=0") || errs != "" {
					t.Errorf("%v: got %q stderr %q, want it taken", a, out, errs)
				}
			}
		})
	}
}

// And the column this one does not speak for: a keyed letter over a frozen
// name holding **nothing** is taken whichever way it is answered, which is
// what makes it the complement of the axis above rather than a widening of
// it.
func TestAKeyedLetterOverAValuelessFrozenNameIsOutsideTheRefusal(t *testing.T) {
	const src = `readonly q; typeset -gA q; echo st=$?`
	for _, a := range []Answer{Yes, No} {
		out, errs, _ := frozenKeyedRun(t, src, a)
		if !strings.Contains(out, "st=0") || errs != "" {
			t.Errorf("%v: got %q stderr %q, want it taken", a, out, errs)
		}
	}
}

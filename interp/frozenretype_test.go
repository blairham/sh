// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A declaration utility's own `name=(…)` operand over a frozen name — see
// Semantics.ArrayLiteralOperandRetypesAFrozenScalar and #2250 — and the
// doubled sentence the same shape used to write.
//
// Tests name axes and wordings, never shells.

func retypeRun(t *testing.T, src string, a Answer) (string, string, int) {
	t.Helper()
	return declRun(t, src, func(s *Semantics) {
		s.DeclareOptions = "aAgiprx"
		s.TypesetLocalNeedsKeywordFunction = No
		s.ArrayLiteralOperandRetypesAFrozenScalar = a
	}, Diagnostics{ReadonlyVariable: "%s: readonly variable"})
}

// The headline, both ways: under Yes the literal replaces the frozen scalar
// and the script carries on, under No it is refused and the name is untouched.
func TestAnArrayLiteralOperandRetypingAFrozenScalarIsAnAxis(t *testing.T) {
	const src = `readonly q=1; typeset -g q=(b); typeset -p q; echo tail`
	out, errs, st := retypeRun(t, src, Yes)
	if want := "declare -ar q=([0]=\"b\")\ntail\n"; out != want || st != 0 || errs != "" {
		t.Errorf("yes: got %q/%d stderr %q, want %q", out, st, errs, want)
	}
	out, errs, st = retypeRun(t, src, No)
	if strings.Contains(out, "tail") || !strings.Contains(errs, "q: readonly variable") {
		t.Errorf("no: got %q/%d stderr %q, want the refusal and no tail", out, st, errs)
	}
}

// The freeze stays on. A retype that quietly unfroze the name would satisfy
// the row above and hand the script a writable name.
func TestARetypedFrozenNameIsStillFrozen(t *testing.T) {
	out, errs, _ := retypeRun(t, "readonly q=1; typeset -g q=(b)\ntypeset -p q\nq=z\n", Yes)
	if !strings.Contains(errs, "q: readonly variable") {
		t.Errorf("stderr %q, want the later assignment still refused", errs)
	}
	if !strings.Contains(out, `q=([0]="b")`) {
		t.Errorf("out %q, want the array the declaration stored", out)
	}
}

// The retype half. A name already holding an array refuses the same operand,
// which is what says the exemption is about leaving the scalar kind rather
// than about the operand's spelling.
func TestAFrozenArrayStillRefusesTheLiteral(t *testing.T) {
	out, errs, _ := retypeRun(t, `readonly q=(a); typeset -g q=(b); echo tail`, Yes)
	if strings.Contains(out, "tail") || !strings.Contains(errs, "q: readonly variable") {
		t.Errorf("got %q stderr %q, want the refusal and no tail", out, errs)
	}
}

// And a declared array holding nothing is an array too, which is the row that
// would pass for the wrong reason against a guard that asked whether the name
// held a value.
func TestAFrozenEmptyArrayStillRefusesTheLiteral(t *testing.T) {
	out, errs, _ := retypeRun(t,
		`typeset -ga e=(); readonly e; typeset -g e=(b); echo tail`, Yes)
	if strings.Contains(out, "tail") || !strings.Contains(errs, "e: readonly variable") {
		t.Errorf("got %q stderr %q, want the refusal and no tail", out, errs)
	}
}

// The declaration half. A bare assignment carrying the same parentheses is not
// a declaration's operand and keeps the refusal it had.
func TestABareArrayLiteralOverAFrozenScalarIsStillRefused(t *testing.T) {
	out, errs, _ := retypeRun(t, `readonly q=1; q=(b); echo tail`, Yes)
	if strings.Contains(out, "tail") || !strings.Contains(errs, "q: readonly variable") {
		t.Errorf("got %q stderr %q, want the refusal and no tail", out, errs)
	}
}

// A scalar operand under the same builtin keeps it too, so the exemption is
// the literal's and not the builtin's.
func TestAScalarDeclarationOverAFrozenNameIsStillRefused(t *testing.T) {
	_, errs, _ := retypeRun(t, `readonly q=1; typeset -g q=b`, Yes)
	if !strings.Contains(errs, "q: readonly variable") {
		t.Errorf("stderr %q, want the refusal", errs)
	}
}

// The sentence is written once. Under No the same line used to report twice
// for a frozen name holding nothing — once for the empty the valueless branch
// stored and once for the array the operand landed — where a frozen name
// holding a value reported once.
func TestTheRefusedLiteralOperandIsReportedOnce(t *testing.T) {
	for _, src := range []string{
		`readonly q; typeset -g q=(b)`,
		`readonly q=1; typeset -g q=(b)`,
	} {
		_, errs, _ := retypeRun(t, src, No)
		if n := strings.Count(errs, "q: readonly variable"); n != 1 {
			t.Errorf("%s: %d refusals in %q, want 1", src, n, errs)
		}
	}
}

// A declaration with no value at all still brings the name into being, which
// is the control for the guard that stopped the valueless branch running for a
// literal operand: it must not have stopped it for a name that carries none.
func TestAValuelessDeclarationStillSetsTheName(t *testing.T) {
	out, _, _ := declRun(t, `typeset -g v; typeset -p v`, func(s *Semantics) {
		s.DeclareOptions = "aAgiprx"
		s.DeclaredNameWithoutValueIsEmpty = Yes
	}, Diagnostics{})
	if want := `declare -- v=""` + "\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// An unanswered axis refuses rather than picking a shell, and only where the
// shape asks it: a frozen scalar meeting a declaration's array literal.
func TestAnUnansweredRetypeAxisRefuses(t *testing.T) {
	_, errs, st := retypeRun(t, `readonly q=1; typeset -g q=(b)`, Unspecified)
	if st == 0 || !strings.Contains(errs, "array literal") {
		t.Errorf("got %d stderr %q, want the unanswered refusal", st, errs)
	}
}

// And nothing else asks it. A line with no frozen name in it must reach no
// question, or every array literal in every script would meet one.
func TestAnUnfrozenArrayLiteralAsksTheRetypeAxisNothing(t *testing.T) {
	out, errs, st := retypeRun(t, `typeset -g q=(b); typeset -p q`, Unspecified)
	if want := `declare -a q=([0]="b")` + "\n"; out != want || st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}

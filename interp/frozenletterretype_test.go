// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A declaration whose numeric type letter lands on a frozen name — see
// Semantics.NumericTypeLetterRetypesAFrozenName and #2539 — and the
// inconsistent-type refusal a type letter stands down, which is the core fault
// the axis alone would have hidden.
//
// Tests name axes and wordings, never shells.

func letterRetypeRun(t *testing.T, src string, a Answer) (string, string, int) {
	t.Helper()
	return declRun(t, src, func(s *Semantics) {
		s.DeclareOptions = "aAFgiprxul"
		s.DeclareOptionsTakingANumber = "Fi"
		s.TypesetLocalNeedsKeywordFunction = No
		// The valueless row needs the standing text re-read for there to be
		// a store to be exempt from, and the row is the shell's own — see
		// AttributeRereadsTheValueItFinds.
		s.AttributeRereadsTheValueItFinds = Yes
		s.NumericTypeLetterRetypesAFrozenName = a
	}, Diagnostics{ReadonlyVariable: "%s: readonly variable"})
}

// The headline, both ways and for both letters: under Yes the declaration
// carries out its own assignment over the freeze and the script carries on,
// under No it is refused and the name keeps what it had.
func TestANumericTypeLetterRetypingAFrozenNameIsAnAxis(t *testing.T) {
	for _, letter := range []string{"i", "F"} {
		src := "readonly q=1; typeset -g" + letter + " q=4; typeset -p q; echo tail"
		out, errs, st := letterRetypeRun(t, src, Yes)
		if !strings.Contains(out, "tail") || st != 0 || errs != "" {
			t.Errorf("-%s yes: got %q/%d stderr %q, want the declaration taken", letter, out, st, errs)
		}
		if !strings.Contains(out, "q=") || strings.Contains(out, `q="1"`) {
			t.Errorf("-%s yes: got %q, want the new value stored", letter, out)
		}
		out, errs, st = letterRetypeRun(t, src, No)
		if strings.Contains(out, "tail") || !strings.Contains(errs, "q: readonly variable") {
			t.Errorf("-%s no: got %q/%d stderr %q, want the refusal and no tail", letter, out, st, errs)
		}
	}
}

// The retype half. A name already carrying the very type the letter names is
// not being retyped, so the ordinary refusal stands — the row that separates
// this from "a type letter is a free hand over a frozen name".
func TestATypeLetterOverANameAlreadyThatTypeIsStillRefused(t *testing.T) {
	out, errs, _ := letterRetypeRun(t,
		`typeset -ir q=1; typeset -gi q=4; typeset -p q; echo tail`, Yes)
	if strings.Contains(out, "tail") || !strings.Contains(errs, "q: readonly variable") {
		t.Errorf("got %q stderr %q, want the refusal and no tail", out, errs)
	}
}

// And the other direction of the same row: integer to float *is* a retype, so
// a guard that asked only whether the name carried some numeric type would
// refuse a line the shell takes.
func TestAFrozenIntegerIsRetypedByTheFloatLetter(t *testing.T) {
	out, errs, st := letterRetypeRun(t,
		`typeset -ir q=1; typeset -gF q=4; typeset -p q; echo tail`, Yes)
	if !strings.Contains(out, "tail") || st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want the retype taken", out, st, errs)
	}
}

// The freeze stays on. A retype that quietly unfroze the name would satisfy
// every row above and hand the script a writable name.
func TestALetterRetypedFrozenNameIsStillFrozen(t *testing.T) {
	_, errs, _ := letterRetypeRun(t, "readonly q=1; typeset -gi q=4\nq=9\n", Yes)
	if !strings.Contains(errs, "q: readonly variable") {
		t.Errorf("stderr %q, want the later assignment still refused", errs)
	}
}

// A declaration with no value at all is exempt too: the standing text is
// re-read through the attribute, which reaches the same store the assignment
// does — so the exemption has to cover the whole operand and not only the
// branch that carries a value.
//
// Only the Yes side is asserted, and deliberately: what a *valueless*
// attribute declaration over a frozen name costs where the retype is refused
// is a question of its own that nothing here models. Measured 2026-09-12,
// `readonly q=1; typeset -i q` is `typeset: q: readonly variable` in bash
// 5.3.15 and `typeset -r -i q=1` in ksh93u+, and this engine takes it in both
// — a divergence that predates this axis and is not what it decides.
func TestAValuelessTypeLetterOverAFrozenNameIsExemptToo(t *testing.T) {
	out, errs, st := letterRetypeRun(t, `readonly q=1; typeset -gi q; typeset -p q`, Yes)
	if st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want the declaration taken", out, st, errs)
	}
	if !strings.Contains(out, `q="1"`) {
		t.Errorf("out %q, want the standing value re-read and kept", out)
	}
}

// A declaration naming no type keeps the refusal it had, so the exemption is
// the letter's rather than the builtin's.
func TestAPlainDeclarationOverAFrozenNameIsStillRefused(t *testing.T) {
	out, errs, _ := letterRetypeRun(t, `readonly q=1; typeset -g q=4; echo tail`, Yes)
	if strings.Contains(out, "tail") || !strings.Contains(errs, "q: readonly variable") {
		t.Errorf("got %q stderr %q, want the refusal and no tail", out, errs)
	}
}

// Two names on one line are two questions. The frozen one is retyped and the
// unfrozen one is declared, which is the row a single flag set for the whole
// builtin would pass by exempting both.
func TestOnlyTheFrozenNameOnTheLineIsExempt(t *testing.T) {
	out, errs, st := letterRetypeRun(t,
		"readonly f=1\ntypeset -gi f s=2\ntypeset -p f\ntypeset -p s\n", Yes)
	if st != 0 || errs != "" {
		t.Errorf("got %d stderr %q, want both operands declared", st, errs)
	}
	if !strings.Contains(out, "f=") || !strings.Contains(out, "s=") {
		t.Errorf("out %q, want both names listed", out)
	}
}

// An unanswered axis refuses rather than picking a shell.
func TestAnUnansweredLetterRetypeAxisRefuses(t *testing.T) {
	_, errs, st := letterRetypeRun(t, `readonly q=1; typeset -gi q=4`, Unspecified)
	if st == 0 || !strings.Contains(errs, "numeric type letter") {
		t.Errorf("got %d stderr %q, want the unanswered refusal", st, errs)
	}
}

// And nothing else asks it. A type letter over a name nobody froze must reach
// no question at all, or every `typeset -i` in every script would meet one.
func TestAnUnfrozenTypeLetterAsksTheRetypeAxisNothing(t *testing.T) {
	out, errs, st := letterRetypeRun(t, `typeset -gi q=4; typeset -p q`, Unspecified)
	if st != 0 || errs != "" || !strings.Contains(out, "q=") {
		t.Errorf("got %q/%d stderr %q, want the ordinary declaration", out, st, errs)
	}
}

// The core half, and it has nothing to do with the freeze: a declaration whose
// letter names a type is not the plain word ScalarOverACompoundIsAnInconsistentType
// refuses over a compound cell. Written against both values of *that* axis so
// the row cannot pass by the refusal being off.
func TestATypeLetterIsNotAPlainWordOverACompound(t *testing.T) {
	run := func(src string, inconsistent Answer) (string, string, int) {
		t.Helper()
		return declRun(t, src, func(s *Semantics) {
			s.DeclareOptions = "aAFgiprxul"
			s.DeclareOptionsTakingANumber = "Fi"
			s.TypesetLocalNeedsKeywordFunction = No
			s.ScalarAssignedOverACompoundReplacesTheName = Yes
			s.ScalarOverACompoundIsAnInconsistentType = inconsistent
		}, Diagnostics{InconsistentType: "%s: inconsistent type for assignment"})
	}
	out, errs, st := run(`typeset -ga q=(a); typeset -gi q=4; typeset -p q; echo tail`, Yes)
	if st != 0 || errs != "" || !strings.Contains(out, "tail") {
		t.Errorf("letter: got %q/%d stderr %q, want the conversion taken", out, st, errs)
	}
	out, errs, _ = run(`typeset -ga q=(a); typeset -g q=4; echo tail`, Yes)
	if strings.Contains(out, "tail") || !strings.Contains(errs, "inconsistent type") {
		t.Errorf("plain: got %q stderr %q, want the refusal and no tail", out, errs)
	}
	out, errs, st = run(`typeset -ga q=(a); typeset -g q=4; echo tail`, No)
	if st != 0 || errs != "" || !strings.Contains(out, "tail") {
		t.Errorf("off: got %q/%d stderr %q, want no refusal at all", out, st, errs)
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The letters that say what a name's *values are* — the integer letter, the
// float letter and the two case letters — and the two questions they raise
// together: whether one of them replaces another on the same name, and
// whether one of them may stand on a declaration whose value is an array
// literal.
//
// Named for the axes and never for a shell. See
// Semantics.NumericAttributeReplacesTheCaseAttribute,
// Semantics.CaseAttributeReplacesTheNumericAttribute and
// Semantics.TypeLetterAndAnArrayLiteralIsAnInconsistentType.

func typeLetterRun(t *testing.T, src string, set func(*Semantics)) (string, string, int) {
	t.Helper()
	return declRun(t, src, func(s *Semantics) {
		s.DeclareOptions = "aAgilprsuxF"
		s.LocalOptions = "aAilprux"
		s.TypesetLocalNeedsKeywordFunction = No
		s.DeclareOptionsTakingANumber = "Fi"
		s.DeclareListing = DeclareListingClustered
		// The case letters fold on the way in, and every element of a
		// compound goes through them: answered so that the rows about a
		// letter beside an array literal reach a value rather than a
		// refusal.
		s.CaseAttributeFoldsWhenRead = No
		s.CompoundElementsGoThroughTheAttribute = Yes
		set(s)
	}, Diagnostics{})
}

// A numeric letter over a case attribute, both ways. The listing is the whole
// of what the two answers differ about, so it is what is read.
func TestANumericLetterReplacingTheCaseAttributeIsAnAxis(t *testing.T) {
	const src = `typeset z=1; typeset -l z; typeset -i z; typeset -p z`
	out, _, _ := typeLetterRun(t, src, func(s *Semantics) {
		s.NumericAttributeReplacesTheCaseAttribute = Yes
	})
	if want := "declare -i z=\"1\"\n"; out != want {
		t.Errorf("yes: got %q, want %q", out, want)
	}
	out, _, _ = typeLetterRun(t, src, func(s *Semantics) {
		s.NumericAttributeReplacesTheCaseAttribute = No
	})
	if want := "declare -il z=\"1\"\n"; out != want {
		t.Errorf("no: got %q, want %q", out, want)
	}
}

// The float letter is the same family and asks the same axis, which is what
// makes it one question rather than one per letter.
func TestTheFloatLetterReplacesTheCaseAttributeToo(t *testing.T) {
	const src = `typeset v=1; typeset -u v; typeset -F 2 v; typeset -p v`
	out, _, _ := typeLetterRun(t, src, func(s *Semantics) {
		s.NumericAttributeReplacesTheCaseAttribute = Yes
	})
	if strings.Contains(out, "u") {
		t.Errorf("yes: got %q, want no case letter left", out)
	}
	out, _, _ = typeLetterRun(t, src, func(s *Semantics) {
		s.NumericAttributeReplacesTheCaseAttribute = No
	})
	if !strings.Contains(out, "u") {
		t.Errorf("no: got %q, want the case letter kept", out)
	}
}

// The other direction is its own axis, and the two are independent: this runs
// with the first answered No so that only the second can be what moves.
func TestACaseLetterReplacingTheNumericAttributeIsAnAxis(t *testing.T) {
	const src = `typeset y=1; typeset -i y; typeset -l y; typeset -p y`
	out, _, _ := typeLetterRun(t, src, func(s *Semantics) {
		s.NumericAttributeReplacesTheCaseAttribute = No
		s.CaseAttributeReplacesTheNumericAttribute = Yes
	})
	if want := "declare -l y=\"1\"\n"; out != want {
		t.Errorf("yes: got %q, want %q", out, want)
	}
	out, _, _ = typeLetterRun(t, src, func(s *Semantics) {
		s.NumericAttributeReplacesTheCaseAttribute = No
		s.CaseAttributeReplacesTheNumericAttribute = No
	})
	if want := "declare -il y=\"1\"\n"; out != want {
		t.Errorf("no: got %q, want %q", out, want)
	}
}

// What goes is the *attribute* and not only the letter in a listing, which a
// listing alone would not show: a name the case letter has untyped stores the
// text of the next assignment rather than evaluating it.
func TestTheCaseLetterTakesTheIntegerReadingWithTheLetter(t *testing.T) {
	const src = `typeset -i n; typeset -l n; n=5+5; echo "[$n]"`
	out, _, _ := typeLetterRun(t, src, func(s *Semantics) {
		s.CaseAttributeReplacesTheNumericAttribute = Yes
	})
	if want := "[5+5]\n"; out != want {
		t.Errorf("yes: got %q, want the plain text", out)
	}
	out, _, _ = typeLetterRun(t, src, func(s *Semantics) {
		s.CaseAttributeReplacesTheNumericAttribute = No
	})
	if want := "[10]\n"; out != want {
		t.Errorf("no: got %q, want the evaluated value", out)
	}
}

// The plus form is not this question: taking a letter off is already its own
// rule, and a declaration that removes one must not be read as adding it.
func TestThePlusFormDoesNotReplaceTheOtherLetter(t *testing.T) {
	out, _, _ := typeLetterRun(t, `typeset z=1; typeset -l z; typeset +i z; typeset -p z`,
		func(s *Semantics) { s.NumericAttributeReplacesTheCaseAttribute = Yes })
	if want := "declare -l z=\"1\"\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// A type letter beside an array literal, both ways. Under Yes the script ends
// there, which a status alone would not show — `tail` never runs.
func TestATypeLetterOverAnArrayLiteralIsAnAxis(t *testing.T) {
	const src = `typeset -ia z=(1 2); echo "st=$? [${z[*]}]"; echo tail`
	out, errs, st := typeLetterRun(t, src, func(s *Semantics) {
		s.TypeLetterAndAnArrayLiteralIsAnInconsistentType = Yes
	})
	if out != "" || st == 0 {
		t.Errorf("yes: got %q/%d, want the script ended with nothing written", out, st)
	}
	if !strings.Contains(errs, "z: inconsistent type for assignment") {
		t.Errorf("yes: stderr %q, want the name and the complaint", errs)
	}
	out, errs, st = typeLetterRun(t, src, func(s *Semantics) {
		s.TypeLetterAndAnArrayLiteralIsAnInconsistentType = No
	})
	if want := "st=0 [1 2]\ntail\n"; out != want || st != 0 || errs != "" {
		t.Errorf("no: got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}

// The array letter is not the trigger, which is what says this is about the
// *type* and not about a pairing: the same line without `-a` is refused, and
// a case letter with `-a` is taken.
func TestTheArrayLetterIsNotWhatTheTypeRefusalTurnsOn(t *testing.T) {
	out, errs, st := typeLetterRun(t, `typeset -i z=(1 2); echo tail`,
		func(s *Semantics) { s.TypeLetterAndAnArrayLiteralIsAnInconsistentType = Yes })
	if out != "" || st == 0 || !strings.Contains(errs, "z: inconsistent type for assignment") {
		t.Errorf("no array letter: got %q/%d stderr %q, want the refusal", out, st, errs)
	}
	out, errs, st = typeLetterRun(t, `typeset -ua q=(ab cd); echo "[${q[*]}]"`,
		func(s *Semantics) { s.TypeLetterAndAnArrayLiteralIsAnInconsistentType = Yes })
	if want := "[AB CD]\n"; out != want || st != 0 || errs != "" {
		t.Errorf("case letter: got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}

// It is the letter on **this line** and not the attribute the name carries: a
// name already declared integer takes an array literal from a later
// declaration that does not write the letter again.
func TestAStandingTypeAttributeDoesNotRefuseALaterLiteral(t *testing.T) {
	out, errs, st := typeLetterRun(t, `typeset -i z; typeset z=(1 2); echo "[${z[*]}]"`,
		func(s *Semantics) { s.TypeLetterAndAnArrayLiteralIsAnInconsistentType = Yes })
	if want := "[1 2]\n"; out != want || st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}

// A plain word is not an array literal, which is the control the rule turns
// on: `typeset -i z=5` is the ordinary integer declaration everywhere.
func TestATypeLetterOverAPlainWordIsNotRefused(t *testing.T) {
	out, errs, st := typeLetterRun(t, `typeset -i z=5+5; echo "[$z]"`,
		func(s *Semantics) { s.TypeLetterAndAnArrayLiteralIsAnInconsistentType = Yes })
	if want := "[10]\n"; out != want || st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}

// The plus form takes the letter off rather than naming a type, so it raises
// no question either.
func TestThePlusFormOfATypeLetterTakesTheLiteral(t *testing.T) {
	out, errs, st := typeLetterRun(t, `typeset +i z=(1 2); echo "[${z[*]}]"`,
		func(s *Semantics) { s.TypeLetterAndAnArrayLiteralIsAnInconsistentType = Yes })
	if want := "[1 2]\n"; out != want || st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}

// The other declaration word that reads these letters refuses it in its own
// name, which is what says the rule belongs to the declaration utilities
// rather than to one word. `local` is the second of the two: `readonly` and
// `export` do not read a type letter at all here, so there is nothing of
// theirs to ask.
func TestTheOtherDeclarationWordRefusesATypeLetterOverALiteral(t *testing.T) {
	out, errs, st := typeLetterRun(t, `f(){ local -i z=(1 2); }; f; echo tail`,
		func(s *Semantics) { s.TypeLetterAndAnArrayLiteralIsAnInconsistentType = Yes })
	if out != "" || st == 0 || !strings.Contains(errs, "z: inconsistent type for assignment") {
		t.Errorf("got %q/%d stderr %q, want the refusal", out, st, errs)
	}
}

// The dialect's own wording reaches the sentence, with the name in it — the
// same one ScalarOverACompoundIsAnInconsistentType writes, which is why there
// is one of it.
func TestTheTypeLetterRefusalUsesTheDialectsWording(t *testing.T) {
	_, errs, _ := declRun(t, `typeset -ia z=(1 2)`, func(s *Semantics) {
		s.DeclareOptions = "aAgilprux"
		s.TypeLetterAndAnArrayLiteralIsAnInconsistentType = Yes
	}, Diagnostics{InconsistentType: "%s is two things at once"})
	if !strings.Contains(errs, "z is two things at once") {
		t.Errorf("stderr %q, want the dialect's wording", errs)
	}
}

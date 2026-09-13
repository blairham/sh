// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `export` reading the declaration letters, where the dialect says it takes
// them — see Semantics.ExportOptions and #2175.
//
// Tests name axes and wordings, never shells.

func exportLettersRun(t *testing.T, src, letters string) (string, string, int) {
	t.Helper()
	return declRun(t, src, func(s *Semantics) {
		s.DeclareOptions = "aAgiprxu"
		s.ExportOptions = letters
		s.IntegerAttributeTakesABase = No
	}, Diagnostics{})
}

// The headline: a type letter this dialect gives `export` is read, and the
// name carries the attribute the letter names as well as the export.
func TestExportReadsTheDeclarationLettersTheDialectGivesIt(t *testing.T) {
	out, errs, st := exportLettersRun(t, `export -i q=2+3; typeset -p q`, "ipru")
	if want := "declare -ix q=\"5\"\n"; out != want || st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}

// And with no letters offered the same line is a bad option, which is the
// answer every other dialect keeps.
func TestExportRefusesADeclarationLetterTheDialectWithholds(t *testing.T) {
	out, errs, st := exportLettersRun(t, `export -i q=4; typeset -p q`, "")
	if st == 0 || out != "" || !strings.Contains(errs, "-i") {
		t.Errorf("got %q/%d stderr %q, want the letter refused", out, st, errs)
	}
}

// A letter outside the offered set is still a bad option, so the field is a
// set and not a switch that opens the whole declaration vocabulary.
func TestExportStillRefusesALetterOutsideTheOfferedSet(t *testing.T) {
	_, errs, st := exportLettersRun(t, `export -a q=4`, "ipru")
	if st == 0 || !strings.Contains(errs, "-a") {
		t.Errorf("got %d stderr %q, want -a refused", st, errs)
	}
}

// It declares a global, which is what the word means here: a lettered
// `export` inside a function must not leave a local behind.
func TestALetteredExportDeclaresAGlobal(t *testing.T) {
	out, _, _ := exportLettersRun(t,
		`f() { export -i k=3; }; f; typeset -p k`, "ipru")
	if want := "declare -ix k=\"3\"\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// The freezing letter freezes, through the same machinery the declaration
// builtin uses rather than a second copy of it.
func TestALetteredExportFreezesTheName(t *testing.T) {
	out, errs, _ := exportLettersRun(t, "export -r q=1\ntypeset -p q\nq=2\n", "ipru")
	if !strings.Contains(out, `q="1"`) || !strings.Contains(errs, "readonly") {
		t.Errorf("out %q stderr %q, want the value and the later refusal", out, errs)
	}
}

// A line with none of the letters on it is the command it always was. The
// route is taken by the letters and not by the field being set, so the
// listing and the plain assignment keep their own shapes.
func TestAnExportWithNoDeclarationLetterIsUnchanged(t *testing.T) {
	out, errs, st := exportLettersRun(t, `export a=1; export -p`, "ipru")
	if st != 0 || errs != "" || !strings.Contains(out, "a=") {
		t.Errorf("got %q/%d stderr %q, want the listing", out, st, errs)
	}
	if strings.Contains(out, "declare") {
		t.Errorf("out = %q, want the export listing rather than a declaration's", out)
	}
}

// A letter with no names is not a declaration of nothing: the route needs
// something to declare, so the line goes to the listing path exactly as it
// did before the field existed.
//
// What that path makes of it is its own business and is not this change's —
// the shell this is for answers with a listing *narrowed* to the names
// carrying the attribute, which this engine does not build for `typeset -i`
// either, and refusing the letter is the honest answer until it does. The
// claim here is only that the declaration route declined the line.
func TestALetteredExportWithNoNamesDeclaresNothing(t *testing.T) {
	out, errs, st := exportLettersRun(t, `export a=1; export -i`, "ipru")
	if st == 0 || out != "" || !strings.Contains(errs, "-i") {
		t.Errorf("got %q/%d stderr %q, want the listing path's refusal", out, st, errs)
	}
}

// A letter inside a *value* is not an option and must not put the line on the
// declaration route. `export t=-i` assigns the two characters.
func TestALetterInsideAValueIsNotAnOption(t *testing.T) {
	out, errs, st := exportLettersRun(t, `export t=-i; typeset -p t`, "ipru")
	if want := "declare -x t=\"-i\"\n"; out != want || st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}

// And neither is one after `--`.
func TestALetterAfterTheEndOfOptionsIsNotAnOption(t *testing.T) {
	out, _, st := exportLettersRun(t, `export -- t=-i; typeset -p t`, "ipru")
	if want := "declare -x t=\"-i\"\n"; out != want || st != 0 {
		t.Errorf("got %q/%d, want %q", out, st, want)
	}
}

// Several names on one lettered line each get the attribute, which is the
// declaration's loop and not a first-operand special case.
func TestALetteredExportAttributesEveryName(t *testing.T) {
	out, _, _ := exportLettersRun(t,
		`export -i q=4 w=5; typeset -p q; typeset -p w`, "ipru")
	want := "declare -ix q=\"4\"\ndeclare -ix w=\"5\"\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// A plus word carrying some other letter does not take the export off. The
// word itself asked for the attribute, so only that word could give it up —
// and the letter that spells it is one this route's dialect refuses outright.
func TestAPlusWordOnALetteredExportStillExports(t *testing.T) {
	out, _, _ := exportLettersRun(t, `export +i q=4; typeset -p q`, "ipru")
	if want := "declare -x q=\"4\"\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// And the plus still does what it was written for: the type letter comes off.
func TestAPlusWordOnALetteredExportStillRemovesTheType(t *testing.T) {
	out, _, _ := exportLettersRun(t,
		"typeset -i q=5\nexport +i q\ntypeset -p q\n", "ipru")
	if strings.Contains(out, "-i") {
		t.Errorf("got %q, want the integer letter gone", out)
	}
	if !strings.Contains(out, "-x") {
		t.Errorf("got %q, want the export kept", out)
	}
}

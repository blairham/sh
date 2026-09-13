// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The `M` letter under the reading that makes it a character mapping. Tests
// name the axis and never a shell — interp/declaremapping.go has the
// measurements.

func withMapping(s *Semantics) {
	s.DeclareOptions = "aAfgilMmprux"
	s.DeclareMappingLetter = DeclareMappingLetterNamesACharacterMapping
	s.DeclaredNameWithoutValueIsEmpty = Yes
	// The mapping *is* the case attribute, so a dialect asked about one is
	// asked about the other: without this the rows below stop at an
	// unanswered axis rather than at the mapping.
	s.AttributeRereadsTheValueItFinds = Yes
}

var mappingDiagnostics = Diagnostics{
	DeclareUnknownMapping:    "%[1]s: %[2]s: unknown mapping name",
	DeclareMappingNeedsAName: "%[1]s: -M requires argument when operands are specified",
	BuiltinUsage:             map[string]string{"typeset": "Usage: typeset"},
}

// The mapping is the attribute letter under another name, in both spellings of
// where the name is written.
func TestAMappingIsTheAttributeLetterUnderAnotherName(t *testing.T) {
	for _, tc := range []struct{ name, line, want string }{
		{"detached", "typeset -M tolower v", "[abc]\n"},
		{"attached", "typeset -Mtolower v", "[abc]\n"},
		{"the other one", "typeset -M toupper v", "[ABC]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "v=AbC\n" + tc.line + "\necho \"[$v]\""
			out, errs, st := declRun(t, src, withMapping, mappingDiagnostics)
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", tc.line, out, errs, st, tc.want)
			}
		})
	}
}

// The attribute holds for every later write, which is what says the mapping
// was recorded as one rather than applied to the value standing there.
func TestAMappingReachesALaterAssignment(t *testing.T) {
	src := "v=AbC\ntypeset -M tolower v\nv=XyZ\necho \"[$v]\""
	out, errs, st := declRun(t, src, withMapping, mappingDiagnostics)
	if want := "[xyz]\n"; out != want || errs != "" || st != 0 {
		t.Errorf("a later assignment = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// A name outside the two is refused and named.
func TestAnUnknownMappingIsNamed(t *testing.T) {
	_, errs, st := declRun(t, "v=1\ntypeset -M nosuch v", withMapping, mappingDiagnostics)
	if want := "testsh: typeset: nosuch: unknown mapping name\n"; errs != want || st != 1 {
		t.Errorf("an unknown mapping = stderr %q status %d, want %q at 1", errs, st, want)
	}
}

// A `--` puts the operands out of the letter's reach, so the name is missing
// and the line is refused. The control is the same line without the dashes,
// where `nosuch` *is* the name and the complaint is the other one.
func TestADoubleDashTakesTheNameAwayFromTheLetter(t *testing.T) {
	_, errs, st := declRun(t, "typeset -M -- nosuch v", withMapping, mappingDiagnostics)
	if want := "testsh: typeset: -M requires argument when operands are specified\n"; errs != want || st != 1 {
		t.Errorf("`-M --` = stderr %q status %d, want %q at 1", errs, st, want)
	}
	_, errs, _ = declRun(t, "typeset -M nosuch v", withMapping, mappingDiagnostics)
	if !strings.Contains(errs, "unknown mapping name") {
		t.Errorf("the same line without the dashes = stderr %q, want the mapping named", errs)
	}
}

// A bare letter is a listing of the mapped names, and this engine keeps no
// mapping apart from the attribute, so it is silent at 0 rather than refused.
func TestTheBareMappingLetterIsSilent(t *testing.T) {
	out, errs, st := declRun(t, "typeset -M\necho done", withMapping, mappingDiagnostics)
	if want := "done\n"; out != want || errs != "" || st != 0 {
		t.Errorf("a bare `typeset -M` = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The function letter refuses the line — but *after* the mapping name has been
// judged, which is the order measured and the only thing that tells the two
// refusals apart on a `-f` line.
func TestTheFunctionLetterIsRefusedAfterTheNameIsJudged(t *testing.T) {
	for _, tc := range []struct {
		name, line, want string
		status           int
	}{
		{"a known mapping", "typeset -fM tolower v", "testsh: Usage: typeset\n", 2},
		{"an unknown one", "typeset -fM nosuch v", "testsh: typeset: nosuch: unknown mapping name\n", 1},
		{"no name at all", "typeset -fM", "testsh: Usage: typeset\n", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, errs, st := declRun(t, "v=1\n"+tc.line, withMapping, mappingDiagnostics)
			if errs != tc.want || st != tc.status {
				t.Errorf("%s = stderr %q status %d, want %q at %d", tc.line, errs, st, tc.want, tc.status)
			}
		})
	}
}

// The rest of the word is a mapping name only under this reading. Where the
// letter means the other thing it is more letters, and a refusal of one of
// them has to survive — which is the row a swallowed name would have run.
func TestTheRestOfTheWordIsLettersUnderTheOtherReading(t *testing.T) {
	other := func(s *Semantics) {
		withMapping(s)
		s.DeclareMappingLetter = DeclareMappingLetterRegistersAMathFunction
	}
	_, errs, st := declRun(t, "v=1\ntypeset -Mzz v", other, mappingDiagnostics)
	if !strings.Contains(errs, "z") || st == 0 {
		t.Errorf("`typeset -Mzz` under the other reading = stderr %q status %d, want the letter refused", errs, st)
	}
}

// A dialect that spells the letter and has not said which reading it means.
func TestTheMappingLetterWithNoReadingChosenIsRefusedByName(t *testing.T) {
	unanswered := func(s *Semantics) {
		withMapping(s)
		s.DeclareMappingLetter = DeclareMappingLetterUnspecified
	}
	_, errs, st := declRun(t, "v=1\ntypeset -M tolower v", unanswered, mappingDiagnostics)
	if !strings.Contains(errs, "the `M` letter of a declaration") ||
		!strings.Contains(errs, "no dialect was chosen") || st != 2 {
		t.Errorf("an unanswered `M` = stderr %q status %d, want the axis named at 2", errs, st)
	}
}

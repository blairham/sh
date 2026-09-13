// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A case letter written on the same declaration as a numeric type letter, and
// two case letters written on one declaration — see #2541,
// Semantics.UpperCaseLetterBesideANumericTypeLetterRecordsNothing and
// Semantics.TwoCaseLettersOnOneDeclarationCancel.
//
// Tests name axes and wordings, never shells.

func caseLetterRun(t *testing.T, src string, set func(*Semantics)) (string, string, int) {
	t.Helper()
	return declRun(t, src, func(s *Semantics) {
		s.DeclareOptions = "aAFgilprux"
		s.DeclareOptionsTakingANumber = "Fi"
		s.TypesetLocalNeedsKeywordFunction = No
		s.UpperCaseLetterBesideANumericTypeLetterRecordsNothing = No
		s.TwoCaseLettersOnOneDeclarationCancel = No
		set(s)
	}, Diagnostics{})
}

// The core half, and it answers to no axis: a case letter beside a numeric
// type letter is *recorded*, in either order of the two. This engine dropped
// it outright, so every column listed a bare `-i` where all three shells list
// the case letter too.
func TestACaseLetterBesideANumericTypeLetterIsRecorded(t *testing.T) {
	for _, src := range []string{
		`typeset -li v=4; typeset -p v`,
		`typeset -il v=4; typeset -p v`,
		`typeset -l -i v=4; typeset -p v`,
		`typeset -i -l v=4; typeset -p v`,
	} {
		out, errs, st := caseLetterRun(t, src, func(*Semantics) {})
		if st != 0 || errs != "" {
			t.Fatalf("%s: got %d stderr %q", src, st, errs)
		}
		// The whole listing, not a letter looked for inside it: `declare`
		// carries an `l` of its own, so a Contains on the bare letter is a
		// probe that cannot fail.
		if want := "declare -il v=\"4\"\n"; out != want {
			t.Errorf("%s: got %q, want %q", src, out, want)
		}
	}
}

// And the value is still the numeric letter's. A case attribute that folded
// the text before the arithmetic saw it would leave the expression standing.
func TestANumericLetterBesideACaseLetterStillEvaluates(t *testing.T) {
	out, _, _ := caseLetterRun(t, `typeset -li i=3+4; echo "[$i]"`, func(*Semantics) {})
	if out != "[7]\n" {
		t.Errorf("got %q, want [7]", out)
	}
}

// The upper-case letter beside a numeric type letter, both ways. Under Yes it
// records nothing and under No it records like any other letter — and the
// lower-case letter is the control, because a rule that dropped the family
// would pass a row that only looked at `-u`.
func TestTheUpperLetterBesideANumericTypeLetterIsAnAxis(t *testing.T) {
	yes := func(s *Semantics) { s.UpperCaseLetterBesideANumericTypeLetterRecordsNothing = Yes }
	out, _, _ := caseLetterRun(t, `typeset -ui v=4; typeset -p v`, yes)
	if want := "declare -i v=\"4\"\n"; out != want {
		t.Errorf("yes: got %q, want %q", out, want)
	}
	out, _, _ = caseLetterRun(t, `typeset -ui v=4; typeset -p v`, func(*Semantics) {})
	if want := "declare -iu v=\"4\"\n"; out != want {
		t.Errorf("no: got %q, want %q", out, want)
	}
	out, _, _ = caseLetterRun(t, `typeset -li v=4; typeset -p v`, yes)
	if want := "declare -il v=\"4\"\n"; out != want {
		t.Errorf("control: got %q, want %q", out, want)
	}
}

// A later declaration is a different question, and the shell that drops the
// letter on one line records it on two. The row that says the axis is about
// one command rather than about the pair of attributes.
func TestTheUpperLetterOnALaterDeclarationIsNotThatAxis(t *testing.T) {
	out, _, _ := caseLetterRun(t, "typeset -i v=4\ntypeset -u v\ntypeset -p v\n",
		func(s *Semantics) {
			s.UpperCaseLetterBesideANumericTypeLetterRecordsNothing = Yes
			s.CaseAttributeReplacesTheNumericAttribute = No
		})
	if want := "declare -iu v=\"4\"\n"; out != want {
		t.Errorf("got %q, want %q — the later line records the letter", out, want)
	}
}

// Two case letters on one declaration, both ways: under Yes neither is
// recorded and the value is not folded, under No the later one speaks.
func TestTwoCaseLettersOnOneDeclarationIsAnAxis(t *testing.T) {
	cancel := func(s *Semantics) { s.TwoCaseLettersOnOneDeclarationCancel = Yes }
	for _, src := range []string{
		`typeset -lu z=Ab; typeset -p z; echo "[$z]"`,
		`typeset -ul z=Ab; typeset -p z; echo "[$z]"`,
		`typeset -l -u -l z=Ab; typeset -p z; echo "[$z]"`,
	} {
		out, errs, st := caseLetterRun(t, src, cancel)
		if st != 0 || errs != "" {
			t.Fatalf("%s: got %d stderr %q", src, st, errs)
		}
		if want := "declare -- z=\"Ab\"\n[Ab]\n"; out != want {
			t.Errorf("%s: got %q, want %q", src, out, want)
		}
	}
	out, _, _ := caseLetterRun(t, `typeset -lu z=Ab; echo "[$z]"`, func(*Semantics) {})
	if !strings.Contains(out, "[AB]") {
		t.Errorf("no: got %q, want the later letter to speak", out)
	}
}

// The cancel takes a standing attribute off rather than merely declining to
// add one — the row a rule written as "skip both branches" passes only by
// accident, since there is nothing to skip on a fresh name.
func TestTheCancelRemovesAStandingCaseAttribute(t *testing.T) {
	out, _, _ := caseLetterRun(t, "typeset -l z=Ab\ntypeset -lu z=Cd\ntypeset -p z\n",
		func(s *Semantics) { s.TwoCaseLettersOnOneDeclarationCancel = Yes })
	if want := "declare -- z=\"Cd\"\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// The sign is read per letter and not off the word: a letter written under a
// plus is not one of the two that cancel, so the minus one still speaks.
func TestALetterUnderAPlusDoesNotCancel(t *testing.T) {
	out, _, _ := caseLetterRun(t, `typeset +l -u z=Ab; typeset -p z; echo "[$z]"`,
		func(s *Semantics) { s.TwoCaseLettersOnOneDeclarationCancel = Yes })
	if want := "declare -u z=\"AB\"\n[AB]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// A later declaration is a different question here too, and all three columns
// agree on it: the second line's letter simply replaces the first's.
func TestTwoCaseLettersOnTwoDeclarationsDoNotCancel(t *testing.T) {
	out, _, _ := caseLetterRun(t, "typeset -l z=Ab\ntypeset -u z\ntypeset -p z\n",
		func(s *Semantics) { s.TwoCaseLettersOnOneDeclarationCancel = Yes })
	// `ab` and not `AB`: the first line's letter folded the value it stored,
	// and the second records an attribute without re-reading what stands —
	// which is AttributeRereadsTheValueItFinds' question and not this one's.
	if want := "declare -u z=\"ab\"\n"; out != want {
		t.Errorf("got %q, want %q — the later letter replaces the first", out, want)
	}
}

// Both axes refuse rather than picking a shell where they are unanswered, and
// only the shapes that ask them meet a question — a declaration with one case
// letter and no numeric one must reach neither.
func TestTheCaseLetterAxesAreAskedOnlyWhereTheyDiffer(t *testing.T) {
	out, errs, st := caseLetterRun(t, `typeset -l z=Ab; typeset -p z`, func(s *Semantics) {
		s.UpperCaseLetterBesideANumericTypeLetterRecordsNothing = Unspecified
		s.TwoCaseLettersOnOneDeclarationCancel = Unspecified
	})
	if want := "declare -l z=\"ab\"\n"; out != want || st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want %q", out, st, errs, want)
	}
	_, errs, st = caseLetterRun(t, `typeset -lu z=Ab`, func(s *Semantics) {
		s.TwoCaseLettersOnOneDeclarationCancel = Unspecified
	})
	if st == 0 || !strings.Contains(errs, "both case letters") {
		t.Errorf("cancel: got %d stderr %q, want the unanswered refusal", st, errs)
	}
	_, errs, st = caseLetterRun(t, `typeset -ui v=4`, func(s *Semantics) {
		s.UpperCaseLetterBesideANumericTypeLetterRecordsNothing = Unspecified
	})
	if st == 0 || !strings.Contains(errs, "upper-case letter") {
		t.Errorf("upper: got %d stderr %q, want the unanswered refusal", st, errs)
	}
}

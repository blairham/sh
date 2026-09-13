// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The other reading of the `m` letter: the operands are `new=old` and the
// parameter moves. Tests name the axis and never a shell — see
// interp/declaremove.go for where each row was measured.

// withMoving is declRun's setter for a dialect whose `m` moves. The letter set
// is the same one withMatching hands out, so the two readings are told apart
// by the axis alone and not by which letters exist.
func withMoving(s *Semantics) {
	s.DeclareOptions = "aAfgilmprux"
	s.DeclareMatchingLetter = DeclareMatchingLetterMoves
	s.BareTypesetListing = BareLocalListsEveryParameter
	s.DeclaredNameWithoutValueIsEmpty = Yes
}

// The move itself, and the source left unset — which is the half that says
// this is a move rather than a copy.
func TestMovingTakesTheValueAndLeavesTheSourceUnset(t *testing.T) {
	src := "qa=1\ntypeset -m qb=qa\necho \"[${qa-UNSET}][${qb-UNSET}]\""
	out, errs, st := declRun(t, src, withMoving, Diagnostics{})
	if want := "[UNSET][1]\n"; out != want || errs != "" || st != 0 {
		t.Errorf("typeset -m qb=qa = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The array kind travels with the value. The control is the scalar row above:
// a move that carried only the scalar view would leave one element behind.
func TestMovingCarriesTheArray(t *testing.T) {
	src := "qa=(x y z)\ntypeset -m qb=qa\necho \"[${qb[1]}] n=${#qb[@]} left=${#qa[@]}\""
	out, errs, st := declRun(t, src, withMoving, Diagnostics{})
	if want := "[y] n=3 left=0\n"; out != want || errs != "" || st != 0 {
		t.Errorf("moving an array = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// A source nobody defined is not a refusal: it unsets the destination, which
// is the same statement read from the other side.
func TestMovingFromANameThatIsNotThereUnsetsTheDestination(t *testing.T) {
	src := "qb=2\ntypeset -m qb=nosuch\necho \"st=$? [${qb-UNSET}]\""
	out, errs, st := declRun(t, src, withMoving, Diagnostics{})
	if want := "st=0 [UNSET]\n"; out != want || errs != "" || st != 0 {
		t.Errorf("typeset -m qb=nosuch = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// An operand with no `=` names the destination and reads the *source* out of
// that name's value. The row is the whole reason a bad value is blamed as a
// bad name: a reading that took the operand as both halves would move nothing
// and say nothing.
func TestMovingWithNoEqualsReadsTheSourceOutOfTheValue(t *testing.T) {
	src := "qa=hello\nhello=world\ntypeset -m qa\necho \"[${qa-UNSET}][${hello-UNSET}]\""
	out, errs, st := declRun(t, src, withMoving, Diagnostics{})
	if want := "[world][UNSET]\n"; out != want || errs != "" || st != 0 {
		t.Errorf("typeset -m qa = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The two halves are blamed differently: a bad destination quotes the whole
// operand back and a bad source quotes only itself. Both rows in one test
// because it is the *difference* that is the measurement.
func TestMovingBlamesTheWholeOperandForADestinationAndOnlyTheSourceForASource(t *testing.T) {
	dg := Diagnostics{
		BuiltinBadName:           map[string]string{"typeset": "%[1]s: %[2]s: invalid variable name"},
		BuiltinBadNameKeepsValue: true,
	}
	for _, tc := range []struct{ name, line, want string }{
		{"a bad destination", "typeset -m 'q*'=9", "typeset: q*=9: invalid variable name"},
		{"a bad source", "typeset -m qb=1bad", "typeset: 1bad: invalid variable name"},
		{"no equals and a value that is no name", "typeset -m qa", "typeset: 1: invalid variable name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, errs, st := declRun(t, "qa=1\n"+tc.line, withMoving, dg)
			if !strings.Contains(errs, tc.want) || st != 1 {
				t.Errorf("%s = stderr %q status %d, want %q at 1", tc.line, errs, st, tc.want)
			}
		})
	}
}

// Every other attribute letter refuses the line with the builtin's usage block
// and no complaint above it. The control is `-p`, which is measured as neither
// a conflict nor an action: a reading that refused every companion letter
// would fail that row and one that refused none would fail the others.
func TestMovingTakesNoOtherLetterButThePrintOne(t *testing.T) {
	dg := Diagnostics{BuiltinUsage: map[string]string{"typeset": "Usage: typeset"}}
	for _, tc := range []struct {
		name, line, wantErr string
		wantStatus          int
	}{
		{"an attribute letter", "typeset -mx 'q*'", "testsh: Usage: typeset\n", 2},
		{"the letters in two words", "typeset -m -x 'q*'", "testsh: Usage: typeset\n", 2},
		{"the function letter", "typeset -fm 'q*'", "testsh: Usage: typeset\n", 2},
		{"the print letter", "typeset -pm 'q*'", "", 0},
		{"the print letter with an assignment", "typeset -pm 'q*'=9", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRun(t, "qa=1\n"+tc.line, withMoving, dg)
			if errs != tc.wantErr || st != tc.wantStatus || out != "" {
				t.Errorf("%s = %q (stderr %q, status %d), want stderr %q at %d with no output",
					tc.line, out, errs, st, tc.wantErr, tc.wantStatus)
			}
		})
	}
}

// The print letter is silent *and inert*: nothing is assigned and nothing is
// listed. Beside the row above because "status 0 with no output" is also what
// a line that quietly did the move would look like from outside.
func TestMovingUnderThePrintLetterAssignsNothing(t *testing.T) {
	src := "qa=1\nqb=2\ntypeset -pm 'q*'=9\necho \"[$qa][$qb]\""
	out, errs, st := declRun(t, src, withMoving, Diagnostics{})
	if want := "[1][2]\n"; out != want || errs != "" || st != 0 {
		t.Errorf("typeset -pm 'q*'=9 = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// A bad operand stops the line, so the operands after it never move. The
// control is the same two operands the other way about, which both move.
func TestMovingStopsAtTheFirstBadOperand(t *testing.T) {
	dg := Diagnostics{
		BuiltinBadName:           map[string]string{"typeset": "%[1]s: %[2]s: invalid variable name"},
		BuiltinBadNameKeepsValue: true,
	}
	refused := "qa=1\ntypeset -m '1bad=qa' 'qb=qa'\necho \"[${qb-UNSET}]\""
	if _, _, st := declRun(t, refused, withMoving, dg); st != 1 {
		t.Errorf("a refused first operand answered %d, want 1", st)
	}
	ok := "qa=1\ntypeset -m 'qb=qa' 'qc=qb'\necho \"[${qc-UNSET}]\""
	out, errs, st := declRun(t, ok, withMoving, dg)
	if want := "[1]\n"; out != want || errs != "" || st != 0 {
		t.Errorf("two good operands = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The letter with nothing after it does nothing at all, where the *other*
// reading's bare form writes the whole parameter table. The pair is the row:
// one axis value, two answers to the same line.
func TestTheBareLetterIsAListingUnderOneReadingAndSilentUnderTheOther(t *testing.T) {
	const src = "qa=1\ntypeset -m\necho done"
	moving, errs, st := declRun(t, src, withMoving, Diagnostics{})
	if want := "done\n"; moving != want || errs != "" || st != 0 {
		t.Errorf("a bare `typeset -m` under the moving reading = %q (stderr %q, status %d), want %q",
			moving, errs, st, want)
	}
	selecting, _, _ := declRun(t, src, withMatching, Diagnostics{})
	if !strings.Contains(selecting, "qa") {
		t.Errorf("a bare `typeset -m` under the selecting reading = %q, want the table listed", selecting)
	}
}

// A dialect that spells the letter and has not said which reading it means is
// refused by name rather than given one of them.
func TestTheLetterWithNoReadingChosenIsRefusedByName(t *testing.T) {
	unanswered := func(s *Semantics) {
		withMoving(s)
		s.DeclareMatchingLetter = DeclareMatchingLetterUnspecified
	}
	_, errs, st := declRun(t, "qa=1\ntypeset -m qb=qa", unanswered, Diagnostics{})
	if !strings.Contains(errs, "the `m` letter of a declaration") ||
		!strings.Contains(errs, "no dialect was chosen") || st != 2 {
		t.Errorf("an unanswered `m` = stderr %q status %d, want the axis named at 2", errs, st)
	}
}

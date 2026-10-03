// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// typeNoOptionsRun is a `type` with no options at all, as the two shells that
// split over TypeLeadingDashWordAsksForTheBareAnswer have.
func typeNoOptionsRun(t *testing.T, src string, bare Answer) (string, string, int) {
	t.Helper()
	return declRun(t, src, func(s *Semantics) {
		s.TypeEndsOptionsWithDashDash = No
		s.TypePrintsFunctionBody = No
		s.TypeLeadingDashWordAsksForTheBareAnswer = bare
		s.CommandNotFoundStatusIsNotFound = Yes
	}, Diagnostics{
		TypeNotFound: "%[1]s: not found", TypeNotFoundUnprefixed: true,
		TypeNotFoundOnStdout: true, TypeNotFoundStatus: 127,
	})
}

// TestALeadingDashWordIsDroppedOrLookedUp: one answer drops the word and
// writes the rest in `command -v`'s shape, the other looks it up as a name.
func TestALeadingDashWordIsDroppedOrLookedUp(t *testing.T) {
	src := "f(){ :; }\ntype -t f\necho st=$?"
	out, _, _ := typeNoOptionsRun(t, src, Yes)
	if out != "f\nst=0\n" {
		t.Errorf("bare: stdout = %q, want %q", out, "f\nst=0\n")
	}
	out, _, _ = typeNoOptionsRun(t, src, No)
	if out != "-t: not found\nf is a function\nst=127\n" {
		t.Errorf("looked up: stdout = %q", out)
	}
	// Only the first word is dropped: a second is a name, and a name that is
	// nothing is silence and the not-found status.
	out, errs, _ := typeNoOptionsRun(t, "f(){ :; }\ntype -t -t f\necho st=$?", Yes)
	if out != "f\nst=127\n" || errs != "" {
		t.Errorf("second dash word: stdout %q stderr %q", out, errs)
	}
}

// TestAnOrdinaryTypeAsksNothingAboutALeadingDash: a first operand with no
// dash reaches no question, so the axis left unanswered refuses nothing.
func TestAnOrdinaryTypeAsksNothingAboutALeadingDash(t *testing.T) {
	out, errs, _ := typeNoOptionsRun(t, "type cd\necho st=$?", Unspecified)
	if out != "cd is a shell builtin\nst=0\n" || errs != "" {
		t.Errorf("stdout %q stderr %q, want the sentence and nothing asked", out, errs)
	}
}

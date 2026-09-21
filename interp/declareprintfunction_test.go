// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// What a `-p` carrying the **function** letter does with an operand naming a
// name that is not there — see
// Semantics.DeclarePrintReportsAMissingFunctionName, which carries the panel.
//
// The field beside it cannot answer this: one column reports a missing
// *variable* and not a missing function, so a single answer would be wrong
// for it whichever way it was set. Tests name the axes, never the shells.

// printsFunctions answers the neighbors a `-pf` needs to arrive at all: the
// two letters on the word, and the variable spelling's own report, which is
// the control here rather than this suite's subject.
func printsFunctions(reports, variables Answer) func(*Semantics) {
	return func(s *Semantics) {
		s.DeclarePrintReportsAMissingFunctionName = reports
		s.DeclarePrintReportsAMissingName = variables
		s.DeclareOptions = "fFgpx"
	}
}

func TestDeclarePrintCanReportAMissingFunctionName(t *testing.T) {
	const src = "g() { :; }\ntypeset -pf nosuch\necho \"st=$?\""
	out, errs, st := declRun(t, src, printsFunctions(Yes, Yes), Diagnostics{})
	if want := "st=1\n"; out != want || st != 0 {
		t.Errorf("reporting: stdout %q status %d, want %q at 0", out, st, want)
	}
	if !strings.Contains(errs, "nosuch: not found") {
		t.Errorf("reporting: stderr %q, want the missing-name sentence", errs)
	}

	out, errs, st = declRun(t, src, printsFunctions(No, Yes), Diagnostics{})
	if want := "st=1\n"; out != want || st != 0 || errs != "" {
		t.Errorf("silent: stdout %q stderr %q status %d, want %q at 0 with nothing said", out, errs, st, want)
	}
}

// The `-p` word is what reports, so the same operand without it is silent on
// both answers — which is what keeps the axis from being read as "the
// function letter reports".
func TestTheFunctionLetterAloneNeverReportsAMissingName(t *testing.T) {
	for _, answer := range []Answer{Yes, No} {
		out, errs, st := declRun(t, "typeset -f nosuch\necho \"st=$?\"",
			printsFunctions(answer, Yes), Diagnostics{})
		if want := "st=1\n"; out != want || errs != "" || st != 0 {
			t.Errorf("answer %v: stdout %q stderr %q status %d, want %q with nothing said",
				answer, out, errs, st, want)
		}
	}
}

// A name that is there is still listed, and a missing one beside it is
// reported on its own — the row that says the report is per operand rather
// than a refusal of the command.
func TestAFoundFunctionListsBesideAMissingOne(t *testing.T) {
	out, errs, st := declRun(t, "g() { :; }\ntypeset -pf g nosuch\necho \"st=$?\"",
		printsFunctions(Yes, Yes), Diagnostics{})
	if !strings.Contains(out, "g ()") {
		t.Errorf("stdout %q, want the found function's body in it", out)
	}
	if !strings.Contains(out, "st=1") {
		t.Errorf("stdout %q, want the status of the missing operand", out)
	}
	if !strings.Contains(errs, "nosuch: not found") || strings.Contains(errs, "g: not found") {
		t.Errorf("stderr %q, want only the missing name reported", errs)
	}
	if st != 0 {
		t.Errorf("status %d, want the script to have run on", st)
	}
}

// A listing that collected its own names has no operand to report, so the
// axis is not asked there — a whole-table `-pf` over a shell with no
// functions is a silent 1 whatever the answer is.
func TestAWholeTableFunctionListingAsksNothing(t *testing.T) {
	sem := permissive()
	sem.DeclarePrintReportsAMissingFunctionName = Unspecified
	sem.DeclarePrintReportsAMissingName = Yes
	sem.DeclareOptions = "fFgpx"

	out, st := run(t, "typeset -pf\necho \"st=$?\"", withSem(sem))
	if want := "st=0\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q — the axis must not be reached", out, st, want)
	}
}

// `-p` beside the names-only letter writes a **declaration line** rather
// than the bare name: the word asks for the shape the line could be reissued
// in. The bare form is the control, and the two differ by nothing else.
func TestThePrintWordMakesANamesOnlyListingADeclaration(t *testing.T) {
	out, errs, st := declRun(t, "g() { :; }\ntypeset -F g\ntypeset -pF g",
		printsFunctions(Yes, Yes), Diagnostics{})
	if want := "g\ndeclare -f g\n"; out != want || errs != "" || st != 0 {
		t.Errorf("got %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

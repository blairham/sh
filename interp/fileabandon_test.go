// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// A fatal error and a request to stop unwind the same way, and a boundary that
// gives up one file catches the first and never the second (#1104).
//
// Every assertion here is the *whole* rendered output, lines and diagnostic
// and location together. Asserting the absence of a failure cannot see this
// bug at all: the failing line reports identically either way, and what
// changes is which of the lines *after* it ran.

// abandonSemantics is permissive() with the two answers these tests need to
// reach the questions they are about, and with the fatal status pinned so a
// rendered line can be compared as a whole.
func abandonSemantics(endsTheFileOnly, paramErrorExits Answer) Semantics {
	s := permissive()
	s.FatalErrorStatusIsOne = Yes
	s.FatalErrorEndsBorrowedTextOnly = endsTheFileOnly
	s.ParamErrorIsAnExitRequest = paramErrorExits
	return s
}

// abandonDiagnostics words the two failures these tests raise, and puts a
// location in front of them, so an assertion covers where as well as what.
func abandonDiagnostics() Diagnostics {
	return Diagnostics{
		Location: LocationLineWord,
		// So the assertion covers *which* file the failing line was in, not
		// only which line: a boundary at the wrong depth reports the wrong
		// file, and the shell's own name would hide that.
		LocationNamesTheCurrentFile: true,
		UnboundVariable:             "%s: parameter not set",
		ParamErrorMessage:           "%[1]s: %[2]s",
	}
}

// sourcedFatal is a file whose third line fails and whose fourth would print,
// so a run that reaches IN-AFTER has not given the file up at all.
const sourcedFatal = "echo IN-BEFORE\nset -u\necho X${NOPE}\necho IN-AFTER\n"

// TestAnErrorInASourcedFileEndsThatFileWhenTheAxisSaysSo asserts the whole of
// the answer, including the line the sourcing file runs afterwards and the
// status the builtin reported. That status is what a `||` in a real startup
// reads, so a fix that resumed the sourcing file with 0 would look right here
// and be wrong.
func TestAnErrorInASourcedFileEndsThatFileWhenTheAxisSaysSo(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "p.sh", sourcedFatal)
	out, st := sourceRun(t, dir, ". ./p.sh\necho \"OUT-AFTER st=$?\"\n",
		abandonSemantics(Yes, No), abandonDiagnostics())
	const want = "IN-BEFORE\n" +
		"./p.sh: line 3: NOPE: parameter not set\n" +
		"OUT-AFTER st=1\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0: the shell was not asked to stop", st)
	}
}

// TestAnErrorInASourcedFileEndsTheShellWhenTheAxisSaysSo is the other answer,
// and the assertion is again the whole output: the sourcing file's next line
// must be absent, which is the half a "no diagnostic" check cannot see.
func TestAnErrorInASourcedFileEndsTheShellWhenTheAxisSaysSo(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "p.sh", sourcedFatal)
	out, st := sourceRun(t, dir, ". ./p.sh\necho \"OUT-AFTER st=$?\"\n",
		abandonSemantics(No, No), abandonDiagnostics())
	const want = "IN-BEFORE\n" +
		"./p.sh: line 3: NOPE: parameter not set\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if st != 1 {
		t.Errorf("status = %d, want the fatal status 1", st)
	}
}

// TestTheStatusASourcedFileFailedWithIsItsOwnField: neither shell that catches
// reports the status the error itself carried, and they disagree with each
// other, so the override has to be reachable and has to be the *only* thing
// that changes.
func TestTheStatusASourcedFileFailedWithIsItsOwnField(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "p.sh", sourcedFatal)
	dg := abandonDiagnostics()
	dg.SourcedFatalStatus = 126
	out, _ := sourceRun(t, dir, ". ./p.sh\necho \"OUT-AFTER st=$?\"\n",
		abandonSemantics(Yes, No), dg)
	const want = "IN-BEFORE\n" +
		"./p.sh: line 3: NOPE: parameter not set\n" +
		"OUT-AFTER st=126\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestExitInASourcedFileIsNotCaughtByTheBoundary is the rule the catch must
// not swallow. Both answers to the axis are run because the bug it guards
// against is a boundary that consumes controlExit without asking which kind it
// is holding, and that bug is invisible under the answer that catches nothing.
func TestExitInASourcedFileIsNotCaughtByTheBoundary(t *testing.T) {
	for _, endsTheFileOnly := range []Answer{Yes, No} {
		dir := t.TempDir()
		write(t, dir, "p.sh", "echo IN-BEFORE\nexit 7\necho IN-AFTER\n")
		out, st := sourceRun(t, dir, ". ./p.sh\necho NOT-REACHED\n",
			abandonSemantics(endsTheFileOnly, No), abandonDiagnostics())
		if out != "IN-BEFORE\n" {
			t.Errorf("axis %v: got %q, want the shell to stop at the exit", endsTheFileOnly, out)
		}
		if st != 7 {
			t.Errorf("axis %v: status = %d, want 7", endsTheFileOnly, st)
		}
	}
}

// TestAnErrorEndsOneSourcedFileAndNotTheStack: the boundary is the innermost
// `.`, so a file sourced from a file sourced from the program loses one file
// and the middle one runs the line after its own `.`.
func TestAnErrorEndsOneSourcedFileAndNotTheStack(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "p.sh", sourcedFatal)
	write(t, dir, "m.sh", "echo MID-BEFORE\n. ./p.sh\necho \"MID-AFTER st=$?\"\n")
	out, _ := sourceRun(t, dir, ". ./m.sh\necho \"OUT-AFTER st=$?\"\n",
		abandonSemantics(Yes, No), abandonDiagnostics())
	const want = "MID-BEFORE\n" +
		"IN-BEFORE\n" +
		"./p.sh: line 3: NOPE: parameter not set\n" +
		"MID-AFTER st=1\n" +
		"OUT-AFTER st=0\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestTheBoundaryIsTheRunningDotAndNotTheFileTheTextCameFrom: a function
// declared in a sourced file and called after it ends the shell. The
// alternative reading — that code which *came from* a sourced file is
// protected — would print OUT-AFTER, and is what an implementation keying on
// the frame rather than on the builtin would do.
func TestTheBoundaryIsTheRunningDotAndNotTheFileTheTextCameFrom(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "p.sh", "f() { echo F-BEFORE; set -u; echo X${NOPE}; echo F-AFTER; }\n")
	out, st := sourceRun(t, dir, ". ./p.sh\nf\necho \"OUT-AFTER st=$?\"\n",
		abandonSemantics(Yes, No), abandonDiagnostics())
	if strings.Contains(out, "OUT-AFTER") {
		t.Errorf("got %q, want the shell to end: the `.` had already finished", out)
	}
	if !strings.Contains(out, "F-BEFORE\n") {
		t.Errorf("got %q, want the function to have started", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want the fatal status 1", st)
	}
}

// TestADotInsideAFunctionResumesTheFunction is the same rule read the other
// way: the boundary is wherever the `.` is, so the body carries on at the
// command after it rather than the call unwinding.
func TestADotInsideAFunctionResumesTheFunction(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "p.sh", sourcedFatal)
	out, _ := sourceRun(t, dir,
		"f() { echo F-BEFORE; . ./p.sh; echo \"F-AFTER st=$?\"; }\nf\necho \"OUT-AFTER st=$?\"\n",
		abandonSemantics(Yes, No), abandonDiagnostics())
	const want = "F-BEFORE\n" +
		"IN-BEFORE\n" +
		"./p.sh: line 3: NOPE: parameter not set\n" +
		"F-AFTER st=1\n" +
		"OUT-AFTER st=0\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestTheErrorOperatorIsItsOwnAxisAtTheBoundary: `${x?word}` is an error in
// one dialect and a request to stop in another, and the pair is run with the
// *same* file so the only difference is the answer.
func TestTheErrorOperatorIsItsOwnAxisAtTheBoundary(t *testing.T) {
	const file = "echo IN-BEFORE\necho X${NOPE?msg}\necho IN-AFTER\n"
	dir := t.TempDir()
	write(t, dir, "p.sh", file)

	out, _ := sourceRun(t, dir, ". ./p.sh\necho \"OUT-AFTER st=$?\"\n",
		abandonSemantics(Yes, No), abandonDiagnostics())
	const caught = "IN-BEFORE\n" +
		"./p.sh: line 2: NOPE: msg\n" +
		"OUT-AFTER st=1\n"
	if out != caught {
		t.Errorf("as an error: got %q, want %q", out, caught)
	}

	out, st := sourceRun(t, dir, ". ./p.sh\necho \"OUT-AFTER st=$?\"\n",
		abandonSemantics(Yes, Yes), abandonDiagnostics())
	const request = "IN-BEFORE\n" +
		"./p.sh: line 2: NOPE: msg\n"
	if out != request {
		t.Errorf("as a request to stop: got %q, want %q", out, request)
	}
	if st != 1 {
		t.Errorf("status = %d, want the fatal status 1", st)
	}
}

// TestAnUnansweredSourcedAbandonmentAxisRefuses: the panel splits four to two,
// so a substrate with no dialect chosen has to say so rather than pick. The
// error still ends the shell, which is the answer that loses nothing.
func TestAnUnansweredSourcedAbandonmentAxisRefuses(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "p.sh", sourcedFatal)
	out, _ := sourceRun(t, dir, ". ./p.sh\necho OUT-AFTER\n",
		abandonSemantics(Unspecified, No), abandonDiagnostics())
	if !strings.Contains(out, "an error inside text a special builtin is running ending that text rather than the shell") {
		t.Errorf("got %q, want the axis named", out)
	}
	if !strings.Contains(out, "the shells disagree here and no dialect was chosen") {
		t.Errorf("got %q, want the refusal", out)
	}
	if strings.Contains(out, "OUT-AFTER") {
		t.Errorf("got %q, want no guess about what the sourcing file does", out)
	}
}

// TestAnUnansweredErrorOperatorAxisRefuses is the same for the second axis,
// and it is only reachable once the first one says the file is a boundary —
// which is the whole point of asking it there and not at the operator.
func TestAnUnansweredErrorOperatorAxisRefuses(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "p.sh", "echo IN-BEFORE\necho X${NOPE?msg}\n")
	out, _ := sourceRun(t, dir, ". ./p.sh\necho OUT-AFTER\n",
		abandonSemantics(Yes, Unspecified), abandonDiagnostics())
	if !strings.Contains(out, "`${x?word}` ending the shell rather than the text it is in") {
		t.Errorf("got %q, want the second axis named", out)
	}
}

// TestAnUnsetParameterUnderNounsetNeverReachesTheErrorOperatorAxis is the
// "ask only at the disagreement" half. The two failures are two lines apart in
// the same file and only one of them has a question, so a run over the other
// must be silent about it — otherwise every `set -u` failure in a sourced file
// would print a refusal nobody can act on.
func TestAnUnsetParameterUnderNounsetNeverReachesTheErrorOperatorAxis(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "p.sh", sourcedFatal)
	out, _ := sourceRun(t, dir, ". ./p.sh\necho \"OUT-AFTER st=$?\"\n",
		abandonSemantics(Yes, Unspecified), abandonDiagnostics())
	if strings.Contains(out, "`${x?word}`") {
		t.Errorf("got %q, want nothing about an axis this failure does not reach", out)
	}
	if !strings.Contains(out, "OUT-AFTER st=1") {
		t.Errorf("got %q, want the sourcing file to have carried on", out)
	}
}

// runAndGiveUp runs text the way a front end runs a startup file — one chunk
// on a session that stays open — and then offers the file up, which is what
// driver.sourceText does around every one of them.
func runAndGiveUp(t *testing.T, dir, text string, sem Semantics, dg Diagnostics) (string, *Runner) {
	t.Helper()
	f, err := syntax.Parse(text, syntax.Core())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf,
		Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "testsh",
	})
	r.Vars = map[string]string{"PATH": dir}
	if err := r.RunPart(context.Background(), f); err != nil {
		t.Fatalf("run: %v", err)
	}
	r.GiveUpTheFile()
	return buf.String(), r
}

// TestGiveUpTheFileEndsAFileAndNotTheSession is the boundary a front end
// reading whole files of its own needs — a shell's startup files. Without it,
// one bad line in one startup file cost the whole invocation.
func TestGiveUpTheFileEndsAFileAndNotTheSession(t *testing.T) {
	dir := t.TempDir()
	sem := abandonSemantics(No, No)
	dg := abandonDiagnostics()

	out, r := runAndGiveUp(t, dir, "echo RC-BEFORE\nset -u\necho X${NOPE}\necho RC-AFTER\n", sem, dg)
	const want = "RC-BEFORE\n" +
		"testsh: line 3: NOPE: parameter not set\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if r.Exited() {
		t.Error("Exited() = true after an error was given up; the session should be open")
	}
}

// TestGiveUpTheFileLeavesAnExitAlone: `exit 3` in a startup file exits 3 and
// the files after it are not read. That is measured in every shell that reads
// more than one, and it is the rule this must not quietly repair.
func TestGiveUpTheFileLeavesAnExitAlone(t *testing.T) {
	dir := t.TempDir()
	out, r := runAndGiveUp(t, dir, "echo RC-BEFORE\nexit 3\necho RC-AFTER\n",
		abandonSemantics(No, No), abandonDiagnostics())
	if out != "RC-BEFORE\n" {
		t.Errorf("got %q, want the file to stop at the exit", out)
	}
	if !r.Exited() {
		t.Error("Exited() = false after `exit 3`; the session should be over")
	}
	if r.ExitStatus() != 3 {
		t.Errorf("status = %d, want 3", r.ExitStatus())
	}
}

// TestGiveUpTheFileHonoursTheErrorOperatorAxis: the operator one dialect reads
// as a request to stop is not caught here either, which is measured at this
// boundary as well — the same `${NOPE?msg}` at the top of a startup file lets
// the program run in one shell and ends the shell in the other.
func TestGiveUpTheFileHonoursTheErrorOperatorAxis(t *testing.T) {
	const file = "echo RC-BEFORE\necho X${NOPE?msg}\n"
	dir := t.TempDir()
	if _, r := runAndGiveUp(t, dir, file, abandonSemantics(No, No), abandonDiagnostics()); r.Exited() {
		t.Error("Exited() = true where the operator is an error; the program should still run")
	}
	if _, r := runAndGiveUp(t, dir, file, abandonSemantics(No, Yes), abandonDiagnostics()); !r.Exited() {
		t.Error("Exited() = false where the operator is a request to stop")
	}
}

// runAndGiveUpTheLine runs text the way a repl runs one accepted line — one
// chunk on a session that stays open — and then offers the *line* up, which is
// what repl.runEach does around every one of them.
func runAndGiveUpTheLine(t *testing.T, dir, text string, sem Semantics, dg Diagnostics) (string, *Runner) {
	t.Helper()
	f, err := syntax.Parse(text, syntax.Core())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf,
		Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "testsh",
	})
	r.Vars = map[string]string{"PATH": dir}
	if err := r.RunPart(context.Background(), f); err != nil {
		t.Fatalf("run: %v", err)
	}
	r.GiveUpTheLine()
	return buf.String(), r
}

// TestGiveUpTheLineEndsALineAndNotTheSession is the third site of this
// boundary: a prompt. What a person typed is one unit of input, and an error
// in it costs the unit rather than the session — measured through a
// pseudo-terminal in bash 5.3, zsh 5.9.2, ksh93u+ and dash, all four of which
// print the diagnostic and draw the next prompt (#1124).
func TestGiveUpTheLineEndsALineAndNotTheSession(t *testing.T) {
	dir := t.TempDir()
	out, r := runAndGiveUpTheLine(t, dir, "set -u\necho X${NOPE}\necho LINE-AFTER\n",
		abandonSemantics(No, No), abandonDiagnostics())
	const want = "testsh: line 2: NOPE: parameter not set\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if r.Exited() {
		t.Error("Exited() = true after an error was given up; the session should be open")
	}
}

// TestGiveUpTheLineLeavesAnExitAlone: `exit` at a prompt ends the session in
// every shell, and so does errexit firing. This is the half that keeps the
// catch from making a shell nobody can leave.
func TestGiveUpTheLineLeavesAnExitAlone(t *testing.T) {
	dir := t.TempDir()
	out, r := runAndGiveUpTheLine(t, dir, "echo LINE-BEFORE\nexit 3\n",
		abandonSemantics(No, No), abandonDiagnostics())
	if out != "LINE-BEFORE\n" {
		t.Errorf("got %q, want the line to stop at the exit", out)
	}
	if !r.Exited() {
		t.Error("Exited() = false after `exit 3`; the session should be over")
	}
	if r.ExitStatus() != 3 {
		t.Errorf("status = %d, want 3", r.ExitStatus())
	}
}

// TestGiveUpTheLineDoesNotAskTheErrorOperatorAxis is the **only** thing that
// separates this site from the file one, and so the only test that can tell
// the two calls apart: `${x?word}` is a request to stop at a file boundary in
// the dialect that documents it that way, and is an error at a prompt in every
// shell measured.
//
// Both answers to the axis are run against the same text, so what is asserted
// is that the answer makes no difference here — where the pair in
// TestGiveUpTheFileHonoursTheErrorOperatorAxis asserts that it makes all of
// it.
func TestGiveUpTheLineDoesNotAskTheErrorOperatorAxis(t *testing.T) {
	const line = "echo LINE-BEFORE\necho X${NOPE?msg}\n"
	dir := t.TempDir()
	for _, paramErrorExits := range []Answer{No, Yes, Unspecified} {
		out, r := runAndGiveUpTheLine(t, dir, line,
			abandonSemantics(No, paramErrorExits), abandonDiagnostics())
		const want = "LINE-BEFORE\n" +
			"testsh: line 2: NOPE: msg\n"
		if out != want {
			t.Errorf("axis %v: got %q, want %q", paramErrorExits, out, want)
		}
		if r.Exited() {
			t.Errorf("axis %v: Exited() = true; a prompt keeps the session for this operand", paramErrorExits)
		}
	}
}

// TestACaughtLineDoesNotMakeALaterExitSurvivable is the same guard the file
// boundary has: after a line has been given up over an error, `exit 7` on the
// next one must still end the session. A catch that cleared the control flow
// and left the kind behind would read that exit as one more error.
func TestACaughtLineDoesNotMakeALaterExitSurvivable(t *testing.T) {
	dir := t.TempDir()
	sem, dg := abandonSemantics(No, No), abandonDiagnostics()
	f, err := syntax.Parse("set -u\necho X${NOPE}\n", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	second, err := syntax.Parse("exit 7\n", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "testsh",
	})
	r.Vars = map[string]string{"PATH": dir}
	if err := r.RunPart(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if !r.GiveUpTheLine() {
		t.Fatal("the first line was not given up over its error")
	}
	if err := r.RunPart(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if r.GiveUpTheLine() {
		t.Error("`exit 7` was caught as an error on the line after a caught one")
	}
	if !r.Exited() || r.ExitStatus() != 7 {
		t.Errorf("Exited() = %v at %d, want the session over at 7", r.Exited(), r.ExitStatus())
	}
}

// TestTheSameBoundaryIsAtEvalAndCarriesADifferentStatus: `eval` and `.` are one
// axis and two statuses, which is the arrangement a parse failure in borrowed
// text already has. The override is the file's; evaluated text keeps the status
// the error carried, so setting the field must change one and not the other.
func TestTheSameBoundaryIsAtEvalAndCarriesADifferentStatus(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "p.sh", sourcedFatal)
	dg := abandonDiagnostics()
	dg.SourcedFatalStatus = 126

	out, _ := sourceRun(t, dir, "eval 'echo IN-BEFORE\nset -u\necho X${NOPE}\necho IN-AFTER'\necho \"OUT-AFTER st=$?\"\n",
		abandonSemantics(Yes, No), dg)
	if !strings.Contains(out, "OUT-AFTER st=1") {
		t.Errorf("eval: got %q, want the error's own status 1 and not the file override", out)
	}
	if strings.Contains(out, "IN-AFTER") {
		t.Errorf("eval: got %q, want the evaluated text given up at the failure", out)
	}

	out, _ = sourceRun(t, dir, ". ./p.sh\necho \"OUT-AFTER st=$?\"\n",
		abandonSemantics(Yes, No), dg)
	if !strings.Contains(out, "OUT-AFTER st=126") {
		t.Errorf("dot: got %q, want the file's own 126", out)
	}
}

// TestAGivenUpStatementDoesNotEndBorrowedText is the third thing controlExit
// is not. A statement the shell abandons costs its line and no more, inside
// borrowed text as at the top of a script — so the `echo` on the next line of
// the same eval runs, and so does the caller afterwards.
//
// Run under both answers to the axis, because a boundary that consumed every
// unwind alike would pass under one of them by accident.
func TestAGivenUpStatementDoesNotEndBorrowedText(t *testing.T) {
	for _, endsTheTextOnly := range []Answer{Yes, No} {
		dir := t.TempDir()
		sem := abandonSemantics(endsTheTextOnly, No)
		// The refusal reports and gives up the statement rather than the
		// script, which is the shape this is about.
		sem.ReadonlyReassignmentFatal = No
		sem.ReadonlyReassignmentFatalFromCommandString = No
		dg := abandonDiagnostics()
		dg.ReadonlyVariable = "%s: readonly variable"
		out, _ := sourceRun(t, dir,
			"eval 'echo IN-BEFORE\nreadonly rr=1\nrr=2\necho IN-AFTER'\necho \"OUT-AFTER st=$?\"\n",
			sem, dg)
		const want = "IN-BEFORE\n" +
			"testsh: line 3: rr: readonly variable\n" +
			"IN-AFTER\n" +
			"OUT-AFTER st=0\n"
		if out != want {
			t.Errorf("axis %v: got %q, want %q", endsTheTextOnly, out, want)
		}
	}
}

// TestAGivenUpStatementTakesTheRestOfItsLineInsideBorrowedText is the half of
// the give-up rule that a "does the next line run" test cannot see: what is
// abandoned is the statement *and the rest of its line*, so `rr=2; echo
// SAME-LINE` prints nothing while an `echo` on the line after it runs.
//
// Measured and unanimous among the shells that survive the refusal at all:
// bash 5.3 and bash 3.2 both print NEXT-LINE and never SAME-LINE, from inside
// a sourced file and from inside an eval.
func TestAGivenUpStatementTakesTheRestOfItsLineInsideBorrowedText(t *testing.T) {
	sem := abandonSemantics(Yes, No)
	sem.ReadonlyReassignmentFatal = No
	sem.ReadonlyReassignmentFatalFromCommandString = No
	dg := abandonDiagnostics()
	dg.ReadonlyVariable = "%s: readonly variable"

	const file = "echo IN-BEFORE\nreadonly rr=1\nrr=2; echo SAME-LINE\necho NEXT-LINE\n"
	const want = "IN-BEFORE\n" +
		"./p.sh: line 3: rr: readonly variable\n" +
		"NEXT-LINE\n" +
		"OUT-AFTER st=0\n"
	dir := t.TempDir()
	write(t, dir, "p.sh", file)
	out, _ := sourceRun(t, dir, ". ./p.sh\necho \"OUT-AFTER st=$?\"\n", sem, dg)
	if out != want {
		t.Errorf("sourced: got %q, want %q", out, want)
	}
}

// TestACaughtErrorDoesNotMakeALaterExitSurvivable is what clearing the kind
// alongside the control is for, and it needs two sourced files to reach:
// after one has been given up over an error, `exit 7` in the next one must
// still end the shell. Measured — ksh93 and zsh both exit 7 with nothing after
// the second `.`.
//
// A boundary that cleared only the control flow would leave the mark behind
// and read the exit as one more error, which is the one way this change could
// have turned `exit` into something survivable.
func TestACaughtErrorDoesNotMakeALaterExitSurvivable(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.sh", sourcedFatal)
	write(t, dir, "b.sh", "echo IN2\nexit 7\necho NOT-REACHED\n")
	out, st := sourceRun(t, dir,
		". ./a.sh\necho \"MID st=$?\"\n. ./b.sh\necho NOT-REACHED-EITHER\n",
		abandonSemantics(Yes, No), abandonDiagnostics())
	const want = "IN-BEFORE\n" +
		"./a.sh: line 3: NOPE: parameter not set\n" +
		"MID st=1\n" +
		"IN2\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if st != 7 {
		t.Errorf("status = %d, want 7", st)
	}
}

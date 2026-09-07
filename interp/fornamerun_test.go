// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runStagedLoop runs src under a dialect that carries an unusable loop name to
// the interpreter — syntax.Dialect.ForNameCheckedWhenTheLoopRuns — with the
// axis set to form. Flags rather than a shell, which is what a core test asks.
func runStagedLoop(t *testing.T, src string, form ForNameRunForm) (string, int) {
	t.Helper()
	d := syntax.Core()
	d.Select = true
	d.ForNameCheckedWhenTheLoopRuns = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out bytes.Buffer
	s := PosixSemantics()
	s.ForNameWhenTheLoopRuns = form
	dg := Diagnostics{ForName: "`%[1]s': not a valid identifier", ForNameStatus: 1}
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out, Dialect: &d, Semantics: &s, Diagnostics: &dg})
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out.String(), st
}

const stagedSrc = "for $n in a b; do echo body; done\necho \"reached-after st=$?\"\n"

// The three forms, which are the three answers among the four panel columns
// that reach the loop at all — see ForNameRunForm for the run they come from.
//
// The complaint is the same sentence in all three; only what becomes of the
// script differs, which is the whole reason the wording is one Diagnostics
// field and the consequence is an axis.
func TestTheThreeAnswersToARefusedLoopName(t *testing.T) {
	for _, tc := range []struct {
		form ForNameRunForm
		out  string
		st   int
	}{
		// bash under its own name: the loop reports 1 and the script runs on.
		{ForNameFailsTheLoop, "sh: `$n': not a valid identifier\nreached-after st=1\n", 0},
		// ksh93: the same sentence, and the script ends at the refusal's own
		// status rather than at a fatal error's.
		{ForNameEndsTheScript, "sh: `$n': not a valid identifier\n", 1},
		// bash in POSIX mode: the same sentence, and the script ends at the
		// dialect's *syntax-error* status. 2 here, which is bash's.
		{ForNameEndsTheScriptAsASyntaxError, "sh: `$n': not a valid identifier\n", 2},
	} {
		out, st := runStagedLoop(t, stagedSrc, tc.form)
		if out != tc.out || st != tc.st {
			t.Errorf("%v:\n got %q (status %d)\nwant %q at %d", tc.form, out, st, tc.out, tc.st)
		}
	}
}

// `select` answers the same as `for`, in every form. #1110 recorded ksh93 as
// fatal for `for` and not for `select`; re-measured, both end the script.
func TestTheMenuLoopTakesTheSameAnswer(t *testing.T) {
	const src = "select $n in a b; do echo body; done\necho \"reached-after st=$?\"\n"
	for _, tc := range []struct {
		form ForNameRunForm
		out  string
		st   int
	}{
		{ForNameFailsTheLoop, "sh: `$n': not a valid identifier\nreached-after st=1\n", 0},
		{ForNameEndsTheScript, "sh: `$n': not a valid identifier\n", 1},
		{ForNameEndsTheScriptAsASyntaxError, "sh: `$n': not a valid identifier\n", 2},
	} {
		out, st := runStagedLoop(t, src, tc.form)
		if out != tc.out || st != tc.st {
			t.Errorf("%v:\n got %q (status %d)\nwant %q at %d", tc.form, out, st, tc.out, tc.st)
		}
	}
}

// A redirection on the clause makes the script-ending form not fatal, and
// leaves the other two alone — which is measured on the one shell that answers
// that way and is why the exception rides the form rather than the clause.
//
// The redirection is still *made*, and that half was measured because the
// first guess was wrong: a loop that never runs looked as though it should not
// open its own file. All four panel columns that reach the loop create it.
func TestARedirectionOnTheClauseChangesOnlyTheFatalForm(t *testing.T) {
	dir := t.TempDir()
	src := "for $n in a b; do echo body; done > " + dir + "/out\necho \"reached-after st=$?\"\n"
	// The form that ends the script does not, with a redirection there.
	out, st := runStagedLoop(t, src, ForNameEndsTheScript)
	if want := "sh: `$n': not a valid identifier\nreached-after st=1\n"; out != want || st != 0 {
		t.Errorf("ForNameEndsTheScript with a redirection:\n got %q (status %d)\nwant %q at 0", out, st, want)
	}
	// The POSIX form still does, which is the row that says the exception is
	// one shell's and not a rule about clauses.
	out, st = runStagedLoop(t, src, ForNameEndsTheScriptAsASyntaxError)
	if want := "sh: `$n': not a valid identifier\n"; out != want || st != 2 {
		t.Errorf("the POSIX form with a redirection:\n got %q (status %d)\nwant %q at 2", out, st, want)
	}
	// And the file is there, the redirection having been made before the
	// variable was looked at.
	if _, err := os.Stat(dir + "/out"); err != nil {
		t.Errorf("the redirection was not made: %v", err)
	}
}

// An unanswered axis is refused by name rather than guessed at, which is the
// promise this repository makes everywhere else and which a *form* makes
// reachable: a dialect can turn the grammar flag on and leave this unset.
func TestARefusedLoopNameWithNoAnswerIsRefusedByName(t *testing.T) {
	// Nothing after the loop, so the status is the refusal's own rather than
	// whatever ran next: an unanswered axis reports and leaves the caller to
	// notice, exactly as every other one does.
	out, st := runStagedLoop(t, "for $n in a b; do echo body; done\n", ForNameRunUnspecified)
	if !strings.Contains(out, "the shells disagree here and no dialect was chosen") {
		t.Errorf("got %q, want the axis refused by name", out)
	}
	if !strings.Contains(out, "a loop variable that is not a name") {
		t.Errorf("got %q, want the axis named", out)
	}
	if strings.Contains(out, "body") {
		t.Errorf("got %q, want the loop not to have run", out)
	}
	if st != 2 {
		t.Errorf("status = %d, want 2", st)
	}
	// And no complaint in the dialect's own words, which would be answering
	// a question nobody answered.
	if strings.Contains(out, "not a valid identifier") {
		t.Errorf("got %q, want no wording where there is no answer", out)
	}
}

// POSIX mode moves an answer and does not invent one.
//
// A dialect can turn the grammar flag on and leave the axis unanswered, and
// entering the mode must not quietly give it POSIX's answer to a question its
// shell never took a position on — the refusal by name is the whole promise.
//
// Asserted because the first version of the swap got it wrong in a way no
// preset could show: the guard it used was true in both directions, so it
// decided nothing and the mode answered for every dialect alike.
//
// The mode is entered through SetPosixMode rather than through `set -o posix`,
// and that is not a shortcut — it is the only route here. The core names no
// such option, so a script asking for it by name gets
// `set: posix: invalid option name` and the mode is never entered: the first
// version of this test did exactly that and passed **vacuously**, which the
// mutant above is what caught. SetPosixMode is exported for a front end that
// was invoked as `sh`, which is the same door.
func TestPosixModeDoesNotAnswerAnUnansweredLoopNameAxis(t *testing.T) {
	d := syntax.Core()
	d.ForNameCheckedWhenTheLoopRuns = true
	f, err := syntax.Parse("for $n in a b; do echo body; done\n", d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out bytes.Buffer
	s := PosixSemantics()
	s.ForNameWhenTheLoopRuns = ForNameRunUnspecified
	dg := Diagnostics{ForName: "`%[1]s': not a valid identifier", ForNameStatus: 1}
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out, Dialect: &d, Semantics: &s, Diagnostics: &dg})
	r.SetPosixMode(true)
	if !r.PosixMode() {
		t.Fatal("the mode was not entered; this test asserts nothing without it")
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "the shells disagree here and no dialect was chosen") {
		t.Errorf("got %q, want the axis still refused by name inside POSIX mode", out.String())
	}
	if strings.Contains(out.String(), "not a valid identifier") {
		t.Errorf("got %q, want no wording where the dialect gave no answer", out.String())
	}
}

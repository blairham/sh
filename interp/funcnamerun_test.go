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

// runStagedFuncName runs src under a dialect that carries an unusable
// function name to the interpreter —
// syntax.Dialect.FunctionNameCheckedWhenTheDefinitionRuns — with the axis set
// to form. Flags rather than a shell, which is what a core test asks.
func runStagedFuncName(t *testing.T, src string, form FuncNameRunForm) (string, int) {
	t.Helper()
	d := syntax.Core()
	d.FunctionNameCheckedWhenTheDefinitionRuns = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out bytes.Buffer
	s := PosixSemantics()
	s.FunctionNameWhenTheDefinitionRuns = form
	dg := Diagnostics{FunctionNameInvalid: "`%[1]s': not a valid identifier"}
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out, Dialect: &d, Semantics: &s, Diagnostics: &dg})
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out.String(), st
}

const stagedFuncSrc = "function _p_${w} { echo HI; }\necho \"reached-after st=$?\"\n"

// The three forms, which are the three answers among the two panel columns
// that reach the definition at all — see FuncNameRunForm for the run they
// come from.
//
// The complaint is the same sentence in all three; only what becomes of the
// script differs, which is the whole reason the wording is one Diagnostics
// field and the consequence is an axis.
func TestTheThreeAnswersToARefusedFunctionName(t *testing.T) {
	for _, tc := range []struct {
		form FuncNameRunForm
		out  string
		st   int
	}{
		// bash under its own name: the definition reports 1 and the script
		// runs on, which is the half #1296 was filed on.
		{FuncNameFailsTheDefinition, "sh: `_p_${w}': not a valid identifier\nreached-after st=1\n", 0},
		// ksh93: the same sentence, and the script ends at 1.
		{FuncNameEndsTheScript, "sh: `_p_${w}': not a valid identifier\n", 1},
		// bash in POSIX mode: the same sentence, and the script ends at the
		// dialect's *syntax-error* status. 2 here, which is bash's.
		{FuncNameEndsTheScriptAsASyntaxError, "sh: `_p_${w}': not a valid identifier\n", 2},
	} {
		out, st := runStagedFuncName(t, stagedFuncSrc, tc.form)
		if out != tc.out || st != tc.st {
			t.Errorf("%v:\n got %q (status %d)\nwant %q at %d", tc.form, out, st, tc.out, tc.st)
		}
	}
}

// Nothing is defined, whichever form ran — which is the half the wording
// cannot show. The literal text of `_p_${w}` is `_p_w`, a perfectly good name
// for a different function, and defining that silently at status 0 is the bug
// #1256 removed and this must not bring back.
func TestARefusedFunctionNameDefinesNothing(t *testing.T) {
	const src = "function _p_${w} { echo HI; }\n_p_w 2>/dev/null || echo nolit\n" +
		"_p_ 2>/dev/null || echo noempty\n"
	out, _ := runStagedFuncName(t, src, FuncNameFailsTheDefinition)
	if !strings.Contains(out, "nolit") || !strings.Contains(out, "noempty") {
		t.Errorf("got %q, want neither the literal name nor the prefix defined", out)
	}
	if strings.Contains(out, "HI") {
		t.Errorf("got %q, want the body never to have run", out)
	}
}

// A redirection on the definition changes nothing, and that was measured
// rather than assumed by symmetry with the loop's axis, where it changes
// everything: `for 1x in a b; do :; done > mf` is *not* fatal in ksh93 where
// the bare clause is, and the same shape on a definition still ends the
// script there. So FuncNameEndsTheScript is the narrower of the two values
// and carries no exception.
//
// The **file is not made either**, which is the other half of the same
// measurement and the second place the two constructs part: a loop clause's
// redirection is opened before the variable is looked at, and a definition's
// belongs to the body and waits for a call. Measured 2026-09-10, `function ok
// { :; } > mf` leaves no `mf` in bash, ksh93 or zsh — with a name they accept
// as readily as with one they do not — so this asserts the definition and not
// the refusal.
func TestARedirectionOnTheDefinitionChangesNothing(t *testing.T) {
	dir := t.TempDir()
	src := "function _p_${w} { echo HI; } > " + dir + "/out\necho \"reached-after st=$?\"\n"
	out, st := runStagedFuncName(t, src, FuncNameEndsTheScript)
	if want := "sh: `_p_${w}': not a valid identifier\n"; out != want || st != 1 {
		t.Errorf("a redirected definition:\n got %q (status %d)\nwant %q at 1", out, st, want)
	}
	if _, err := os.Stat(dir + "/out"); !os.IsNotExist(err) {
		t.Errorf("the redirection was made at the definition, where no shell makes it: %v", err)
	}
}

// An unanswered axis is refused by name rather than guessed at, which is the
// promise this repository makes everywhere else and which a *form* makes
// reachable: a dialect can turn the grammar flag on and leave this unset.
func TestARefusedFunctionNameWithNoAnswerIsRefusedByName(t *testing.T) {
	out, st := runStagedFuncName(t, "function _p_${w} { echo HI; }\n", FuncNameRunUnspecified)
	if !strings.Contains(out, "the shells disagree here and no dialect was chosen") {
		t.Errorf("got %q, want the axis refused by name", out)
	}
	if !strings.Contains(out, "a function name that is not a name") {
		t.Errorf("got %q, want the axis named", out)
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

// POSIX mode moves an answer and does not invent one — the same promise the
// loop's axis makes, asserted here because the swap is a second copy of the
// same four lines and a copy is exactly what drifts.
func TestPosixModeDoesNotAnswerAnUnansweredFunctionNameAxis(t *testing.T) {
	d := syntax.Core()
	d.FunctionNameCheckedWhenTheDefinitionRuns = true
	f, err := syntax.Parse("function _p_${w} { echo HI; }\n", d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out bytes.Buffer
	s := PosixSemantics()
	s.FunctionNameWhenTheDefinitionRuns = FuncNameRunUnspecified
	dg := Diagnostics{FunctionNameInvalid: "`%[1]s': not a valid identifier"}
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

// Leaving POSIX mode puts the dialect's own answer back rather than the
// standard's opposite, which is the half a saved field exists for: a shell
// that stops at 1 must not come out of the mode stopping at 2.
func TestLeavingPosixModePutsTheFunctionNameAnswerBack(t *testing.T) {
	d := syntax.Core()
	d.FunctionNameCheckedWhenTheDefinitionRuns = true
	f, err := syntax.Parse(stagedFuncSrc, d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out bytes.Buffer
	s := PosixSemantics()
	s.FunctionNameWhenTheDefinitionRuns = FuncNameEndsTheScript
	dg := Diagnostics{FunctionNameInvalid: "`%[1]s': not a valid identifier"}
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out, Dialect: &d, Semantics: &s, Diagnostics: &dg})
	r.SetPosixMode(true)
	r.SetPosixMode(false)
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if st != 1 {
		t.Errorf("status = %d after leaving the mode, want the dialect's own 1", st)
	}
}

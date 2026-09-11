// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// ArithCommandErrorIsFatal decides what an expression `(( ))` could not
// evaluate does to the rest of the input, and nothing else: the complaint is
// written and the status is the construct's under either answer.
//
// Both ways an expression can fail are here because the axis covers both, the
// same pair ArithCommandErrorStatusIsTwo is asserted over: `(( 1+ ))` never
// reaches the evaluator and `(( 1/0 ))` does.
func TestAnArithmeticCommandStopsTheInputOnlyWhenItIsFatal(t *testing.T) {
	for _, src := range []string{
		`echo one; (( 1+ )); echo two`,
		`echo one; (( 1/0 )); echo two`,
	} {
		sem := PosixSemantics()
		sem.ArithCommandErrorIsFatal = No
		out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
		if !strings.Contains(out, "one") || !strings.Contains(out, "two") {
			t.Errorf("not fatal: %q got %q, want the input to carry on", src, out)
		}

		fatal := PosixSemantics()
		fatal.ArithCommandErrorIsFatal = Yes
		// The syntax status is pinned so that both rows leave the same
		// number and the assertion is about the status surviving the
		// give-up rather than about which branch produced it. The preset's
		// own is 2, which only the parse row would show.
		diag := Diagnostics{SyntaxErrorStatus: 1}
		out, st := run(t, src, func(r *Runner) { r.Semantics = &fatal; r.Diagnostics = &diag })
		if !strings.Contains(out, "one") {
			t.Errorf("fatal: %q got %q, want what came before it to have run", src, out)
		}
		if strings.Contains(out, "two") {
			t.Errorf("fatal: %q got %q, want nothing after it to have run", src, out)
		}
		if st != 1 {
			t.Errorf("fatal: %q status %d, want the construct's own 1", src, st)
		}
	}
}

// It is the *expression* and not what the status is read for: the construct
// standing as a condition is fatal on the same terms.
func TestAnArithmeticConditionIsFatalOnTheSameTerms(t *testing.T) {
	for _, src := range []string{
		`echo one; (( 1+ )) && echo yes; echo two`,
		`echo one; if (( 1+ )); then echo yes; fi; echo two`,
	} {
		sem := PosixSemantics()
		sem.ArithCommandErrorIsFatal = Yes
		out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
		if strings.Contains(out, "two") || strings.Contains(out, "yes") {
			t.Errorf("%q got %q, want the input abandoned at the expression", src, out)
		}
	}
}

// The give-up is an *error*, so a boundary that reads a file of its own gives
// up that file and the caller carries on. Both constructs that ask a fatality
// question go through the one door, which is what this asserts by running both
// through the same boundary.
//
// Marked only as a request to stop, the give-up cost the whole script — the
// same error one level down as the one interp/source.go is named for.
func TestAFatalArithmeticFailureEndsTheBorrowedTextAlone(t *testing.T) {
	for _, line := range []string{`(( 1+ ))`, `[[ 1+ -eq 0 ]]`} {
		dir := t.TempDir()
		script := filepath.Join(dir, "s.sh")
		if err := os.WriteFile(script, []byte(line+"\necho insrc\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		sem := PosixSemantics()
		sem.ArithCommandErrorIsFatal = Yes
		sem.ConditionArithmeticErrorIsFatal = Yes
		sem.FatalErrorEndsBorrowedTextOnly = Yes
		sem.BuiltinSyntaxErrorFatal = No
		sem.DotMissingFileFatal = No
		sem.DotPassesArguments = Yes
		sem.DotFallsBackToCurrentDirectory = No
		sem.DotWithNoOperandIsAnError = Yes
		out, _ := run(t, ". "+script+"; echo after", func(r *Runner) { r.Semantics = &sem })
		if strings.Contains(out, "insrc") {
			t.Errorf("%s: got %q, want the rest of the file abandoned", line, out)
		}
		if !strings.Contains(out, "after") {
			t.Errorf("%s: got %q, want the caller to carry on past the file", line, out)
		}
	}
}

// An unanswered axis is refused by name, and only a failing expression reaches
// it: one that evaluates cleanly never asks.
func TestAnUnansweredArithmeticCommandFatalityIsRefused(t *testing.T) {
	sem := PosixSemantics()
	sem.ArithCommandErrorIsFatal = Unspecified
	if out, _ := run(t, `(( 2 )); echo "ok=$?"`, func(r *Runner) { r.Semantics = &sem }); out != "ok=0\n" {
		t.Errorf("a clean expression got %q, want it never to have asked", out)
	}
	out, _ := run(t, `(( 1+ ))`, func(r *Runner) { r.Semantics = &sem })
	if !strings.Contains(out, "abandoning the input") {
		t.Errorf("got %q, want the unanswered axis named", out)
	}
}

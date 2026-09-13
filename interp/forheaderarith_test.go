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

// ForHeaderArithmeticErrorIsFatal decides what an expression a C-style `for`
// header could not evaluate does to the rest of the input, and nothing else:
// the complaint is written and the status is 1 under either answer.
//
// Measured 2026-09-13 on ksh93u+ 2012-08-01 and zsh 5.9.2 against bash 5.3.15,
// bash 3.2.57 and bash as `sh`. `for ((i=0; i<1/0; i++)); do :; done; echo
// "A st=$?"` prints the complaint and then `A st=1` in the three bash columns,
// ending 0, and prints the complaint and nothing after it in ksh93 and zsh,
// ending 1. dash and BusyBox ash have no C-style `for` at all.
//
// All three parts are here because all three are fatal, and both failure
// branches because both shells give up for both — the same pairing
// ArithCommandErrorStatusIsTwo found one construct over.
func TestAForHeaderStopsTheInputOnlyWhenItIsFatal(t *testing.T) {
	for _, c := range []struct{ name, header string }{
		{"the condition", `for (( i=0; i<1/0; i++ )); do :; done`},
		{"the initializer", `for (( i=1/0;; )); do break; done`},
		// The step runs at the loop's back edge, so a body holding a `break`
		// never reaches it and a row written with one measures nothing. This
		// body runs and the loop is ended by the failure instead.
		{"the step", `for (( i=0; i<3; i=1/0 )); do echo body; done`},
		// A part that never reaches the evaluator. One axis answers for both
		// branches, which is why it is asserted here rather than assumed.
		{"a part that will not parse", `for (( i=0; 1+; i++ )); do :; done`},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := "echo one; " + c.header + "; echo two"

			sem := PosixSemantics()
			sem.ForHeaderArithmeticErrorIsFatal = No
			out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
			if !strings.Contains(out, "one") || !strings.Contains(out, "two") {
				t.Errorf("not fatal: got %q, want the input to carry on", out)
			}

			fatal := PosixSemantics()
			fatal.ForHeaderArithmeticErrorIsFatal = Yes
			out, st := run(t, src, func(r *Runner) { r.Semantics = &fatal })
			if !strings.Contains(out, "one") {
				t.Errorf("fatal: got %q, want what came before it to have run", out)
			}
			if strings.Contains(out, "two") {
				t.Errorf("fatal: got %q, want nothing after it to have run", out)
			}
			if st != 1 {
				t.Errorf("fatal: status %d, want 1", st)
			}
		})
	}
}

// A header that evaluates never reaches the question, which is what keeps
// `for ((;;))` an endless loop rather than a fatal one.
func TestAForHeaderThatEvaluatesIsNotFatal(t *testing.T) {
	sem := PosixSemantics()
	sem.ForHeaderArithmeticErrorIsFatal = Yes
	out, st := run(t, `for (( i=0; i<2; i++ )); do echo body; done; echo two`,
		func(r *Runner) { r.Semantics = &sem })
	if strings.Count(out, "body") != 2 || !strings.Contains(out, "two") {
		t.Errorf("got %q, want two passes and the line after", out)
	}
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}
}

// It is not the question `(( ))` asks, and the two do not cut the panel the
// same way: zsh gives up a header and stays for an arithmetic command. A
// single field could not carry both, so the two answers have to be reachable
// independently — which is what this asserts, in each direction.
func TestTheForHeaderAxisIsNotTheArithmeticCommandsOne(t *testing.T) {
	header := PosixSemantics()
	header.ForHeaderArithmeticErrorIsFatal = Yes
	header.ArithCommandErrorIsFatal = No
	out, _ := run(t, `echo one; (( 1/0 )); echo two`,
		func(r *Runner) { r.Semantics = &header })
	if !strings.Contains(out, "two") {
		t.Errorf("the header axis gave up an arithmetic command: %q", out)
	}

	cmd := PosixSemantics()
	cmd.ForHeaderArithmeticErrorIsFatal = No
	cmd.ArithCommandErrorIsFatal = Yes
	out, _ = run(t, `echo one; for (( i=1/0;; )); do break; done; echo two`,
		func(r *Runner) { r.Semantics = &cmd })
	if !strings.Contains(out, "two") {
		t.Errorf("the arithmetic-command axis gave up a header: %q", out)
	}
}

// The give-up is an *error*, so a boundary reading a file of its own gives up
// that file and the caller carries on — the same door `(( ))` goes through,
// and measured to be the same: ksh93 and zsh both print `after` over a sourced
// file whose only line is a failing header.
func TestAFatalForHeaderEndsTheSourcedFileAlone(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "s.sh")
	if err := os.WriteFile(script, []byte("for (( i=1/0;; )); do :; done\necho insrc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sem := PosixSemantics()
	sem.ForHeaderArithmeticErrorIsFatal = Yes
	// The `.` builtin's own axes, pinned so the row is about the give-up's
	// reach and not about which reading of `.` the preset happens to hold —
	// the same set TestAFatalArithmeticFailureEndsTheBorrowedTextAlone pins.
	sem.FatalErrorEndsBorrowedTextOnly = Yes
	sem.BuiltinSyntaxErrorFatal = No
	sem.DotMissingFileFatal = No
	sem.DotPassesArguments = Yes
	sem.DotFallsBackToCurrentDirectory = No
	sem.DotWithNoOperandIsAnError = Yes
	out, _ := run(t, ". "+script+"; echo after", func(r *Runner) { r.Semantics = &sem })
	if strings.Contains(out, "insrc") {
		t.Errorf("got %q, want the rest of the file abandoned", out)
	}
	if !strings.Contains(out, "after") {
		t.Errorf("got %q, want the sourced file given up and the caller carrying on", out)
	}
}

// TestACleanForHeaderAsksNothing pins where the axis is consulted, the way
// TestOrdinaryShortCircuitingAsksNothing does one construct over.
//
// `ask` on an unanswered axis refuses and takes the status with it, so an axis
// read at the top of the clause would make every C-style `for` in the language
// fail under a vector that has not chosen — including every test whose answer
// set predates the axis, and including `for ((;;))`, which would become fatal
// rather than endless. The two readings agree on a header that evaluates,
// because there is no failure for either of them to be about.
//
// It fails loudly if a later change hoists the ask, which is the whole reason
// it is written down.
func TestACleanForHeaderAsksNothing(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"three parts that evaluate", `for (( i=0; i<2; i++ )); do :; done; echo "ok=$?"`},
		{"a body that breaks out of an endless header", `for ((;;)); do break; done; echo "ok=$?"`},
		{"an empty condition", `for (( i=0;; i++ )); do break; done; echo "ok=$?"`},
	} {
		t.Run(c.name, func(t *testing.T) {
			sem := PosixSemantics()
			sem.ForHeaderArithmeticErrorIsFatal = Unspecified
			out, st := run(t, c.src, func(r *Runner) { r.Semantics = &sem })
			if out != "ok=0\n" || st != 0 {
				t.Errorf("got %q at %d, want %q at 0 and no refusal", out, st, "ok=0\n")
			}
		})
	}
}

// And the other half of the same rule: a header that *does* fail reaches the
// disagreement, so an unanswered vector refuses it by name rather than quietly
// picking a side. Both failure branches, because the axis answers for both.
func TestAFailedForHeaderIsRefusedWhenNothingAnswered(t *testing.T) {
	for _, src := range []string{
		`for (( i=0; i<1/0; i++ )); do :; done`,
		`for (( i=1/0;; )); do break; done`,
		`for (( i=0; i<3; i=1/0 )); do echo body; done`,
		`for (( i=0; 1+; i++ )); do :; done`,
	} {
		sem := PosixSemantics()
		sem.ForHeaderArithmeticErrorIsFatal = Unspecified
		out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
		if !strings.Contains(out, "the shells disagree here and no dialect was chosen") {
			t.Errorf("%s gave %q, want the unanswered axis to be named", src, out)
		}
		if !strings.Contains(out, "a `for (( ))` header that could not be evaluated") {
			t.Errorf("%s gave %q, want the refusal to say which axis", src, out)
		}
	}
}

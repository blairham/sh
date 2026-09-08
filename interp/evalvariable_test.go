// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// Command text a front end read out of a variable, run as `eval` runs its
// arguments and named in a diagnostic by the variable it came out of.

// The text runs in the calling shell, and what it did is still there after.
func TestTextFromAVariableRunsInTheCallingShell(t *testing.T) {
	var out strings.Builder
	sem := interp.PosixSemantics()
	r := newTestRunner(t, &interp.Runner{Semantics: &sem, Vars: map[string]string{}, Stdout: &out, Stderr: &out})

	if st := r.EvalVariable(t.Context(), "PROMPT_COMMAND", `echo ran; x=7`); st != 0 {
		t.Errorf("the text reported %d, want 0", st)
	}

	if got := out.String(); got != "ran\n" {
		t.Errorf("the text printed %q, want %q", got, "ran\n")
	}
	if v, _ := r.GetVar("x"); v != "7" {
		t.Errorf("the assignment left x=%q, want 7 — it ran somewhere else", v)
	}
}

// The text is shown the status the caller was carrying.
//
// Measured, all six columns: `false; eval 'echo $?'` prints 1. It is the whole
// of what a prompt hook is told, and a shell that cleared the status first
// would answer "the last command succeeded" immediately after one did not.
func TestTextFromAVariableIsShownTheCallersStatus(t *testing.T) {
	var out strings.Builder
	sem := interp.PosixSemantics()
	r := newTestRunner(t, &interp.Runner{Semantics: &sem, Vars: map[string]string{}, Stdout: &out, Stderr: &out})
	r.SetExitStatus(5)

	r.EvalVariable(t.Context(), "PROMPT_COMMAND", `echo "st=$?"`)

	if got := out.String(); got != "st=5\n" {
		t.Errorf("the text saw %q, want %q", got, "st=5\n")
	}
}

// A failure is reported against the variable, not against `eval` and not
// against a file.
//
// The name is the person's only way back to the text. Measured, bash writes
// `PROMPT_COMMAND: line N: syntax error …` for exactly this reason.
func TestTextThatWillNotParseIsNamedByItsVariable(t *testing.T) {
	var out strings.Builder
	sem := interp.PosixSemantics()
	sem.BuiltinSyntaxErrorFatal = interp.No
	r := newTestRunner(t, &interp.Runner{Semantics: &sem, Vars: map[string]string{}, Stdout: &out, Stderr: &out})

	r.EvalVariable(t.Context(), "PROMPT_COMMAND", `if`)

	got := out.String()
	if !strings.Contains(got, "PROMPT_COMMAND") {
		t.Errorf("the failure was reported as %q, want the variable named in it", got)
	}
	if strings.Contains(got, "eval") {
		t.Errorf("the failure was reported as %q, want no mention of a builtin nobody ran", got)
	}
}

// Text with nothing in it reports success, which is what an empty `eval` does
// and is a different question from the one above.
func TestTextFromAVariableWithNothingInItReportsSuccess(t *testing.T) {
	sem := interp.PosixSemantics()
	r := newTestRunner(t, &interp.Runner{Semantics: &sem, Vars: map[string]string{}})
	r.SetExitStatus(5)

	if st := r.EvalVariable(t.Context(), "PROMPT_COMMAND", "   \n"); st != 0 {
		t.Errorf("text with no commands in it reported %d, want 0", st)
	}
}

// The variable's name beats the dialect's own word for eval's text.
//
// One dialect calls `eval`'s text `(eval)` rather than `eval`, and text out of
// a variable is neither: it is not a file, and the person did not type `eval`.
// Asserted against a Diagnostics that *has* that word, because the shell with
// `PROMPT_COMMAND` is not the shell with `(eval)` — so nothing else in the
// tree reaches this, and a naming rule nothing reaches is one that quietly
// stops being true.
func TestAVariablesNameBeatsTheDialectsWordForEval(t *testing.T) {
	var out strings.Builder
	sem := interp.PosixSemantics()
	sem.BuiltinSyntaxErrorFatal = interp.No
	d := interp.PosixDiagnostics()
	d.EvalSourceName = "(eval)"
	r := newTestRunner(t, &interp.Runner{
		Semantics: &sem, Diagnostics: &d,
		Vars: map[string]string{}, Stdout: &out, Stderr: &out,
	})

	r.EvalVariable(t.Context(), "PROMPT_COMMAND", `if`)

	got := out.String()
	if !strings.Contains(got, "PROMPT_COMMAND") {
		t.Errorf("the failure was reported as %q, want the variable named in it", got)
	}
	if strings.Contains(got, "(eval)") {
		t.Errorf("the failure was reported as %q, want the variable and not the dialect's word for eval", got)
	}
}

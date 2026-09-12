// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A prompt rendering is a boundary: an error inside its expansion pass costs
// the rendering and not the shell (#2053).
//
// The rule is named as a style answer and an axis, never as a shell — the
// measured bytes of the two dialects that have a prompt language are in
// dialect/zsh and dialect/bash. What is asserted here is that both answers are
// reachable, that the catch happens at all, and that a request to stop still
// is not caught.

// abandonPromptStyle is a stand-in dialect with a prompt language: one escape
// code — the escape character doubled, which every such language has — so the
// table has something to draw and the assertion can see whether it still ran
// over what an abandoned pass left behind.
func abandonPromptStyle(keepsWhatItDrew bool) PromptStyle {
	return PromptStyle{
		Expand:                         PromptExpandsAlways,
		ExpandBeforeEscapes:            true,
		FailedExpansionKeepsWhatItDrew: keepsWhatItDrew,
		Escape:                         '%',
		Codes:                          map[rune]PromptField{'%': FieldEscape},
	}
}

// abandonPromptField answers the one code the stand-in table has and nothing
// else, so a refusal in these tests is a code the test did not mean to write.
func abandonPromptField(f PromptField, _ string, _ bool) (string, bool) {
	if f == FieldEscape {
		return "%", true
	}
	return "", false
}

// renderPrompt renders one value through a runner that has the given answers,
// and reports what was drawn together with whether the runner is still
// willing to run anything afterwards.
//
// Both halves are returned because either one alone passes the bug: the text
// was already right on the day the whole script stopped, and a runner that
// carries on while drawing the wrong thing is the other failure this pair
// exists to tell apart.
func renderPrompt(t *testing.T, value string, keepsWhatItDrew bool, sem Semantics, dg Diagnostics) (drawn, diag string, alive bool) {
	t.Helper()
	var buf bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf,
		Semantics: &sem, Diagnostics: &dg,
		Name: "testsh",
	})
	r.SetPromptStyle(abandonPromptStyle(keepsWhatItDrew))
	out, refused, ok := RenderPromptValue(r.PromptStyleValue(), r, value, abandonPromptField, nil)
	if !ok {
		t.Fatalf("the escape table refused %q at %q, which is not what this test is about", value, refused)
	}
	return out, buf.String(), !r.Exited()
}

// promptSemantics is permissive() with the fatal status pinned, so a rendered
// diagnostic can be compared as a whole line.
func promptSemantics(paramErrorExits Answer) Semantics {
	s := permissive()
	s.FatalErrorStatusIsOne = Yes
	s.ParamErrorIsAnExitRequest = paramErrorExits
	return s
}

func promptDiagnostics() Diagnostics {
	return Diagnostics{
		Location:          LocationLineWord,
		DivisionByZero:    "division by zero",
		UnboundVariable:   "%s: parameter not set",
		ParamErrorMessage: "%[1]s: %[2]s",
	}
}

// TestAGivenUpRenderingIsWorthWhatItDrew is the answer one dialect gives: the
// text in front of the substitution that failed, and nothing after it.
func TestAGivenUpRenderingIsWorthWhatItDrew(t *testing.T) {
	drawn, diag, alive := renderPrompt(t, `PRE-$((1/0))-POST`, true, promptSemantics(No), promptDiagnostics())
	if drawn != "PRE-" {
		t.Errorf("drawn = %q, want %q: what the pass produced before it gave up", drawn, "PRE-")
	}
	if diag != "testsh: line 0: division by zero\n" {
		t.Errorf("diagnostic = %q, want the failure reported once", diag)
	}
	if !alive {
		t.Error("the runner is unwinding: the rendering was given up, not the shell")
	}
}

// TestAGivenUpRenderingKeepsTheTextItWasHanded is the other answer, and it is
// the reason this is a field rather than a rule: the same value, the same
// failure, and a different thing handed back.
func TestAGivenUpRenderingKeepsTheTextItWasHanded(t *testing.T) {
	drawn, _, alive := renderPrompt(t, `PRE-$((1/0))-POST`, false, promptSemantics(No), promptDiagnostics())
	if drawn != `PRE-$((1/0))-POST` {
		t.Errorf("drawn = %q, want the text as it stood", drawn)
	}
	if !alive {
		t.Error("the runner is unwinding: the boundary is the same whatever the value is worth")
	}
}

// TestWhatARenderingDrewStopsAtTheFirstSubstitution asserts the half a walk
// that simply stopped at the failing span would get wrong: a substitution that
// *succeeded* before the failure is thrown away with the rest.
func TestWhatARenderingDrewStopsAtTheFirstSubstitution(t *testing.T) {
	var buf bytes.Buffer
	sem, dg := promptSemantics(No), promptDiagnostics()
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf,
		Semantics: &sem, Diagnostics: &dg,
		Name: "testsh",
	})
	r.SetPromptStyle(abandonPromptStyle(true))
	r.Vars = map[string]string{"V": "MID"}
	drawn, _, ok := RenderPromptValue(r.PromptStyleValue(), r, `PRE-${V}-$((1/0))-POST`, abandonPromptField, nil)
	if !ok {
		t.Fatal("the escape table refused, which is not what this test is about")
	}
	if drawn != "PRE-" {
		t.Errorf("drawn = %q, want %q: MID expanded and is dropped with the rest", drawn, "PRE-")
	}
}

// TestTheEscapeTableStillReadsWhatAGivenUpRenderingDrew: the two passes are not
// both given up. An implementation that returned early from the whole rendering
// would hand back the escape undrawn and pass every other test here.
func TestTheEscapeTableStillReadsWhatAGivenUpRenderingDrew(t *testing.T) {
	drawn, _, _ := renderPrompt(t, `PRE-%%-$((1/0))-POST`, true, promptSemantics(No), promptDiagnostics())
	if drawn != "PRE-%-" {
		t.Errorf("drawn = %q, want %q: the table read what the abandoned pass left", drawn, "PRE-%-")
	}
}

// TestTheErrorOperatorIsNotCaughtAtARenderingWhenTheAxisSaysSo is the half that
// keeps this from catching everything: the one operand a dialect reads as a
// request to stop is not caught here, exactly as it is not caught at a file.
func TestTheErrorOperatorIsNotCaughtAtARenderingWhenTheAxisSaysSo(t *testing.T) {
	_, diag, alive := renderPrompt(t, `PRE-${NOPE?gone}-POST`, true, promptSemantics(Yes), promptDiagnostics())
	if diag != "testsh: line 0: NOPE: gone\n" {
		t.Errorf("diagnostic = %q, want the operand's own word", diag)
	}
	if alive {
		t.Error("the runner carried on: the operator this axis calls a request to stop must not be caught here")
	}
}

// TestTheErrorOperatorIsCaughtAtARenderingWhenTheAxisSaysSo is the other side
// of the same axis, without which the test above passes for a boundary that
// catches nothing at all.
func TestTheErrorOperatorIsCaughtAtARenderingWhenTheAxisSaysSo(t *testing.T) {
	drawn, _, alive := renderPrompt(t, `PRE-${NOPE?gone}-POST`, true, promptSemantics(No), promptDiagnostics())
	if drawn != "PRE-" {
		t.Errorf("drawn = %q, want %q", drawn, "PRE-")
	}
	if !alive {
		t.Error("the runner is unwinding: this answer makes the operand an ordinary error")
	}
}

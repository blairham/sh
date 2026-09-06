// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The per-shell answers the interp tests used to assert inline: the interp
// package proves what each axis value does, and this file pins which value
// this preset gives, so a preset edit cannot silently flip one.

// answersRun parses and runs one snippet as this dialect.
//
// The dialect is handed to the runner as well as to the parser, and both
// halves are load-bearing. The parser decides what the source *is*; the
// runner asks Runner.Dialect what a pattern means, whether arithmetic has
// floats, and what grammar nested input — a command substitution, an `eval`,
// a trap body, a sourced file — is parsed with. A runner built without one
// falls back to the core, so a test whose whole purpose is to assert this
// dialect's answer was asserting the core's: `[[ $k == a(b|c) ]]` parsed here
// and then did not match (#849, found closing #826).
func answersRun(t *testing.T, src string) (string, int) {
	t.Helper()
	d := ksh.Dialect()
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem, diag := ksh.Semantics(), ksh.Diagnostics()
	r := &interp.Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &diag,
		Dialect: &d, Name: "sh", Env: []string{"PATH=/usr/bin:/bin"},
	}
	ksh.Apply(r)
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		return buf.String() + "unsupported: " + rerr.Error(), -1
	}
	return buf.String(), st
}

// TestTheHelperRunsUnderThisDialectAndNotTheCore guards the field answersRun
// sets, which nothing else in this package would miss.
//
// The arithmetic evaluator asks Runner.Dialect directly rather than reading
// anything the parser left behind, and ksh93 is the only panel member that
// evaluates in floating point. Measured, ksh93u+:
//
//	$ ksh -c 'echo $((1.5 + 1))'
//	2.5
//
// Without the field the core answers, and the core refuses the operand:
// `1.5 + 1: arithmetic syntax error`.
func TestTheHelperRunsUnderThisDialectAndNotTheCore(t *testing.T) {
	out, st := answersRun(t, `echo $((1.5 + 1))`)
	if strings.TrimSpace(out) != "2.5" || st != 0 {
		t.Errorf("answersRun = %q status %d, want 2.5 and 0: the runner was not told the dialect",
			out, st)
	}
}

func TestAnswersTheInterpAxisTestsRelyOn(t *testing.T) {
	s := ksh.Semantics()
	for _, tc := range []struct {
		axis string
		got  interp.Answer
		want interp.Answer
	}{
		{"BraceExpansion", s.BraceExpansion, interp.Yes},
		{"BracketCaretNegates", s.BracketCaretNegates, interp.Yes},
		{"EqualsExpansion", s.EqualsExpansion, interp.No},
		{"UnsetPositionalIsAllowed", s.UnsetPositionalIsAllowed, interp.Yes},
		{"ExitTrapIsFunctionLocal", s.ExitTrapIsFunctionLocal, interp.No},
		{"ArithNameValueRecurses", s.ArithNameValueRecurses, interp.Yes},
		{"ArithInvalidOctalDigitIsError", s.ArithInvalidOctalDigitIsError, interp.No},
		{"ArrayBaseIsZero", s.ArrayBaseIsZero, interp.Yes},
		{"ArrayLiteralSubscriptIsAKey", s.ArrayLiteralSubscriptIsAKey, interp.Yes},
		{"FatalErrorStatusIsOne", s.FatalErrorStatusIsOne, interp.Yes},
		{"RedirectErrorOnSpecialBuiltinFatal", s.RedirectErrorOnSpecialBuiltinFatal, interp.Yes},
		{"UnsetReadonlyFatal", s.UnsetReadonlyFatal, interp.No},
		{"MultiDigitDuplicationTargetIsAnError", s.MultiDigitDuplicationTargetIsAnError, interp.No},
		{"SplitParamExpansion", s.SplitParamExpansion, interp.Yes},
		{"GlobNoMatchIsError", s.GlobNoMatchIsError, interp.No},
		{"ReadonlyReassignmentFatal", s.ReadonlyReassignmentFatal, interp.Yes},
		{"ShiftPastEndFatal", s.ShiftPastEndFatal, interp.Yes},
		{"TraceAssignmentsSeparately", s.TraceAssignmentsSeparately, interp.Yes},
		{"TraceShowsItsOwnDisabling", s.TraceShowsItsOwnDisabling, interp.No},
		{"LocalInheritsTheExportAttribute", s.LocalInheritsTheExportAttribute, interp.No},
		{"StdinOptionNamesTheOperands", s.StdinOptionNamesTheOperands, interp.Yes},
		{"HangupIsAnOrderlyExit", s.HangupIsAnOrderlyExit, interp.No},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.axis, tc.got, tc.want)
		}
	}
	if got, want := s.UnterminatedBracket, interp.BracketLiteral; got != want {
		t.Errorf("UnterminatedBracket = %v, want %v", got, want)
	}
	if got, want := s.ExitArgument, interp.ExitArgLenient; got != want {
		t.Errorf("ExitArgument = %v, want %v", got, want)
	}
	if got, want := s.SubshellJobTable, interp.SubshellJobsKept; got != want {
		t.Errorf("SubshellJobTable = %v, want %v", got, want)
	}
}

// The invocation answers this preset gives that are not Answers.
func TestPlusSignedCommandStringNaming(t *testing.T) {
	if got := ksh.Semantics().PlusSignedCommandStringIsDollarZero; got != true {
		t.Errorf("PlusSignedCommandStringIsDollarZero = %v, want true", got)
	}
}

func TestDiagnosticAnswersTheInterpTestsRelyOn(t *testing.T) {
	d := ksh.Diagnostics()
	if got, want := d.TraceQuoting, interp.QuoteDollar; got != want {
		t.Errorf("TraceQuoting = %v, want %v", got, want)
	}
	if got, want := d.TraceStyle, interp.TracePlain; got != want {
		t.Errorf("TraceStyle = %v, want %v", got, want)
	}
	if got, want := d.TraceForHeader, interp.TraceForNone; got != want {
		t.Errorf("TraceForHeader = %v, want %v", got, want)
	}
	if got := d.SyntaxStatus(); got != 3 {
		t.Errorf("SyntaxStatus() = %d, want 3", got)
	}
}

// TestScriptDiagnosticsNameTheScript: under `-c` this shell names a line only
// after the first, and a script names line 1 like any other — the reason
// ScriptLocation exists.
func TestScriptDiagnosticsNameTheScript(t *testing.T) {
	if got := ksh.Diagnostics().Report("s", 2, "m"); got != "s: line 2: m" {
		t.Errorf("-c: %q, want %q", got, "s: line 2: m")
	}
	if got := ksh.Diagnostics().Report("s", 1, "m"); got != "s: m" {
		t.Errorf("-c line 1: %q, want %q", got, "s: m")
	}
	if got := ksh.Diagnostics().ForScript().Report("s", 1, "m"); got != "s: line 1: m" {
		t.Errorf("script line 1: %q, want %q — a script names its first line", got, "s: line 1: m")
	}
	if got := ksh.Diagnostics().ForScript().Report("s", 2, "m"); got != "s: line 2: m" {
		t.Errorf("script: %q, want %q", got, "s: line 2: m")
	}
}

// TestWordings runs the failures whose sentences are this shell's own.
func TestWordings(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"arithmetic", `echo $((1/0))`, "1/0: divide by zero"},
		{"shift", `shift 5`, "shift: 5: bad number"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want it to contain %q", out, tc.want)
			}
		})
	}
}

// A `test` diagnostic is written by the builtin under whichever of its two
// names was typed, so no wording may spell one of them itself.
func TestNoTestWordingSpellsItsOwnName(t *testing.T) {
	d := ksh.Diagnostics()
	for _, w := range []struct{ field, text string }{
		{"TestUnaryExpected", d.TestUnaryExpected},
		{"TestBinaryExpected", d.TestBinaryExpected},
		{"TestIntegerExpected", d.TestIntegerExpected},
		{"TestTooManyArguments", d.TestTooManyArguments},
		{"TestOperandExpected", d.TestOperandExpected},
	} {
		if strings.HasPrefix(w.text, "test:") || strings.HasPrefix(w.text, "[:") {
			t.Errorf("%s = %q: names a builtin that may have been called by its other name", w.field, w.text)
		}
	}
	if got := d.TestMissingBracket; got != "" && !strings.ContainsAny(got, "[]") {
		t.Errorf("TestMissingBracket = %q: want a bracket in it", got)
	}
}

// TestATypesetLocalDoesNotCarryTheExportAttribute: there is no `local` here,
// so the question is asked through `typeset` in a keyword function — where a
// child is told nothing about the shadowed name, and the outer name is
// exported again once the function returns.
func TestATypesetLocalDoesNotCarryTheExportAttribute(t *testing.T) {
	out, _ := answersRun(t, `export FOO=bar; function f { typeset FOO=baz; /usr/bin/env | grep '^FOO=' || echo "(none)"; }; f; /usr/bin/env | grep '^FOO='`)
	if strings.Contains(out, "FOO=baz") {
		t.Errorf("got %q, want the child told nothing under the name", out)
	}
	if !strings.Contains(out, "(none)") || !strings.Contains(out, "FOO=bar") {
		t.Errorf("got %q, want nothing inside and the outer value after", out)
	}
}

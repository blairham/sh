// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The per-shell answers the interp tests used to assert inline: the interp
// package proves what each axis value does, and this file pins which value
// this preset gives, so a preset edit cannot silently flip one.

func answersRun(t *testing.T, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, bash.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem, diag := bash.Semantics(), bash.Diagnostics()
	r := &interp.Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &diag,
		Name: "sh", Env: []string{"PATH=/usr/bin:/bin"},
	}
	bash.Apply(r)
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		return buf.String() + "unsupported: " + rerr.Error(), -1
	}
	return buf.String(), st
}

func TestAnswersTheInterpAxisTestsRelyOn(t *testing.T) {
	s := bash.Semantics()
	for _, tc := range []struct {
		axis string
		got  interp.Answer
		want interp.Answer
	}{
		{"BracketCaretNegates", s.BracketCaretNegates, interp.Yes},
		{"EqualsExpansion", s.EqualsExpansion, interp.No},
		{"LastPipelineElementInCurrentShell", s.LastPipelineElementInCurrentShell, interp.No},
		{"UnsetPositionalIsAllowed", s.UnsetPositionalIsAllowed, interp.No},
		{"ExitTrapIsFunctionLocal", s.ExitTrapIsFunctionLocal, interp.No},
		{"ArithNameValueRecurses", s.ArithNameValueRecurses, interp.Yes},
		{"ArithInvalidOctalDigitIsError", s.ArithInvalidOctalDigitIsError, interp.Yes},
		{"ArrayBaseIsZero", s.ArrayBaseIsZero, interp.Yes},
		{"ArrayLiteralSubscriptIsAKey", s.ArrayLiteralSubscriptIsAKey, interp.No},
		{"FatalErrorStatusIsOne", s.FatalErrorStatusIsOne, interp.Yes},
		{"EchoInterpretsEscapes", s.EchoInterpretsEscapes, interp.No},
		{"LengthOfSpecialIsCount", s.LengthOfSpecialIsCount, interp.Yes},
		{"SplitParamExpansion", s.SplitParamExpansion, interp.Yes},
		{"GlobNoMatchIsError", s.GlobNoMatchIsError, interp.No},
		{"ReadonlyReassignmentFatal", s.ReadonlyReassignmentFatal, interp.No},
		{"ShiftPastEndFatal", s.ShiftPastEndFatal, interp.No},
		{"TraceAssignmentsSeparately", s.TraceAssignmentsSeparately, interp.Yes},
		{"TraceShowsItsOwnDisabling", s.TraceShowsItsOwnDisabling, interp.Yes},
		{"LocalInheritsTheExportAttribute", s.LocalInheritsTheExportAttribute, interp.Yes},
		{"StdinOptionNamesTheOperands", s.StdinOptionNamesTheOperands, interp.No},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.axis, tc.got, tc.want)
		}
	}
	if got, want := s.UnterminatedBracket, interp.BracketLiteral; got != want {
		t.Errorf("UnterminatedBracket = %v, want %v", got, want)
	}
	if got, want := s.ExitArgument, interp.ExitArgNumeric; got != want {
		t.Errorf("ExitArgument = %v, want %v", got, want)
	}
}

// The invocation answers this preset gives that are not Answers.
func TestPlusSignedCommandStringNaming(t *testing.T) {
	if got := bash.Semantics().PlusSignedCommandStringIsDollarZero; got != false {
		t.Errorf("PlusSignedCommandStringIsDollarZero = %v, want false", got)
	}
}

func TestDiagnosticAnswersTheInterpTestsRelyOn(t *testing.T) {
	d := bash.Diagnostics()
	if got, want := d.TraceQuoting, interp.QuoteShell; got != want {
		t.Errorf("TraceQuoting = %v, want %v", got, want)
	}
	if got, want := d.TraceStyle, interp.TracePlain; got != want {
		t.Errorf("TraceStyle = %v, want %v", got, want)
	}
	if got, want := d.TraceForHeader, interp.TraceForSource; got != want {
		t.Errorf("TraceForHeader = %v, want %v", got, want)
	}
	if got, want := d.Location, interp.LocationLineWord; got != want {
		t.Errorf("Location = %v, want %v", got, want)
	}
	if got := d.SyntaxStatus(); got != 2 {
		t.Errorf("SyntaxStatus() = %d, want 2", got)
	}
	// The two Report routes agree here; only one dialect splits them.
	if d.Report("s", 2, "m") != d.ForScript().Report("s", 2, "m") {
		t.Error("Report should not change between -c and a script")
	}
}

// TestWordings runs the failures whose sentences are this shell's own.
func TestWordings(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"readonly", `readonly r=1; r=2`, "r: readonly variable"},
		{"not found", `nosuchcommand_xyz`, "command not found"},
		{"unbound", `set -u; echo "$NOPE"`, "unbound variable"},
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
// names was typed, so no wording may spell one of them itself. Six strings,
// each written separately, and the ones a corpus case does not reach are
// exactly the ones that would go unnoticed — so the claim is made about all
// of them at once rather than about the handful that are easy to run.
func TestNoTestWordingSpellsItsOwnName(t *testing.T) {
	d := bash.Diagnostics()
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
	// The one wording that may spell a bracket, because only one of the two
	// names can reach it: nothing looks for a closing bracket unless one
	// opened.
	if got := d.TestMissingBracket; got != "" && !strings.ContainsAny(got, "[]") {
		t.Errorf("TestMissingBracket = %q: want a bracket in it", got)
	}
}

// TestALocalCarriesTheExportAttribute: a local shadowing an exported name is
// exported itself here, so a child sees the local's value — and the caller's
// value comes back with the caller.
func TestALocalCarriesTheExportAttribute(t *testing.T) {
	out, _ := answersRun(t, `export FOO=bar; f() { local FOO=baz; /usr/bin/env | grep '^FOO=' || echo "(none)"; }; f; /usr/bin/env | grep '^FOO='`)
	if !strings.Contains(out, "FOO=baz") || !strings.Contains(out, "FOO=bar") {
		t.Errorf("got %q, want the local's value inside and the outer value after", out)
	}
}

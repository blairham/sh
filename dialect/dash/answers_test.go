// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The per-shell answers the interp tests used to assert inline: the interp
// package proves what each axis value does, and this file pins which value
// this preset gives, so a preset edit cannot silently flip one.

func answersRun(t *testing.T, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, dash.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem, diag := dash.Semantics(), dash.Diagnostics()
	r := &interp.Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &diag,
		Name: "sh", Env: []string{"PATH=/usr/bin:/bin"},
	}
	dash.Apply(r)
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		return buf.String() + "unsupported: " + rerr.Error(), -1
	}
	return buf.String(), st
}

func TestAnswersTheInterpAxisTestsRelyOn(t *testing.T) {
	s := dash.Semantics()
	for _, tc := range []struct {
		axis string
		got  interp.Answer
		want interp.Answer
	}{
		{"EqualsExpansion", s.EqualsExpansion, interp.No},
		{"LastPipelineElementInCurrentShell", s.LastPipelineElementInCurrentShell, interp.No},
		{"UnsetPositionalIsAllowed", s.UnsetPositionalIsAllowed, interp.No},
		{"ExitTrapIsFunctionLocal", s.ExitTrapIsFunctionLocal, interp.No},
		{"ArithNameValueRecurses", s.ArithNameValueRecurses, interp.No},
		{"ArrayBaseIsZero", s.ArrayBaseIsZero, interp.Yes},
		{"SplitParamExpansion", s.SplitParamExpansion, interp.Yes},
		{"GlobNoMatchIsError", s.GlobNoMatchIsError, interp.No},
		{"ReadonlyReassignmentFatal", s.ReadonlyReassignmentFatal, interp.Yes},
		{"ShiftPastEndFatal", s.ShiftPastEndFatal, interp.Yes},
		{"TraceAssignmentsSeparately", s.TraceAssignmentsSeparately, interp.No},
		{"TraceShowsItsOwnDisabling", s.TraceShowsItsOwnDisabling, interp.Yes},
		{"LocalInheritsTheExportAttribute", s.LocalInheritsTheExportAttribute, interp.Yes},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.axis, tc.got, tc.want)
		}
	}
	if got, want := s.UnterminatedBracket, interp.BracketNoMatch; got != want {
		t.Errorf("UnterminatedBracket = %v, want %v", got, want)
	}
	if got, want := s.ExitArgument, interp.ExitArgStrict; got != want {
		t.Errorf("ExitArgument = %v, want %v", got, want)
	}
}

func TestDiagnosticAnswersTheInterpTestsRelyOn(t *testing.T) {
	d := dash.Diagnostics()
	if got, want := d.TraceQuoting, interp.QuoteNever; got != want {
		t.Errorf("TraceQuoting = %v, want %v", got, want)
	}
	if got, want := d.TraceStyle, interp.TracePlain; got != want {
		t.Errorf("TraceStyle = %v, want %v", got, want)
	}
	if got, want := d.TraceForHeader, interp.TraceForNone; got != want {
		t.Errorf("TraceForHeader = %v, want %v", got, want)
	}
	if got, want := d.Location, interp.LocationColonLine; got != want {
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
		{"readonly", `readonly r=1; r=2`, "r: is read only"},
		{"not found", `nosuchcommand_xyz`, "nosuchcommand_xyz: not found"},
		{"arithmetic", `echo $((1/0))`, `arithmetic expression: division by zero: "1/0"`},
		{"invalid number", `x=abc; echo $((x+1))`, "Illegal number: abc"},
		{"shift", `shift 5`, "shift: can't shift that many"},
		{"unbound", `set -u; echo "$NOPE"`, "parameter not set"},
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
	d := dash.Diagnostics()
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

// TestALocalCarriesTheExportAttribute: the same answer as bash, reached with
// this shell's own reading of a valueless declaration — `local FOO` leaves the
// caller's value showing through, and it is exported, so the child is told
// `FOO=bar` where bash tells it nothing.
func TestALocalCarriesTheExportAttribute(t *testing.T) {
	out, _ := answersRun(t, `export FOO=bar; f() { local FOO=baz; /usr/bin/env | grep '^FOO=' || echo "(none)"; }; f`)
	if !strings.Contains(out, "FOO=baz") {
		t.Errorf("got %q, want the child told the local's value", out)
	}
	out, _ = answersRun(t, `export FOO=bar; f() { local FOO; /usr/bin/env | grep '^FOO=' || echo "(none)"; }; f`)
	if !strings.Contains(out, "FOO=bar") {
		t.Errorf("valueless: got %q, want the outer value showing through", out)
	}
}

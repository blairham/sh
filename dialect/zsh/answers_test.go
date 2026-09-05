// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The per-shell answers the interp tests used to assert inline: the interp
// package proves what each axis value does, and this file pins which value
// this preset gives — plus the few composites that are this shell's alone.

func answersRun(t *testing.T, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, zsh.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem, diag := zsh.Semantics(), zsh.Diagnostics()
	r := &interp.Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &diag,
		Name: "sh", Env: []string{"PATH=/usr/bin:/bin"},
	}
	zsh.Apply(r)
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		return buf.String() + "unsupported: " + rerr.Error(), -1
	}
	return buf.String(), st
}

func TestAnswersTheInterpAxisTestsRelyOn(t *testing.T) {
	s := zsh.Semantics()
	for _, tc := range []struct {
		axis string
		got  interp.Answer
		want interp.Answer
	}{
		{"BraceExpansion", s.BraceExpansion, interp.Yes},
		{"BracketCaretNegates", s.BracketCaretNegates, interp.Yes},
		{"EqualsExpansion", s.EqualsExpansion, interp.Yes},
		{"LastPipelineElementInCurrentShell", s.LastPipelineElementInCurrentShell, interp.Yes},
		{"UnsetPositionalIsAllowed", s.UnsetPositionalIsAllowed, interp.No},
		{"ExitTrapIsFunctionLocal", s.ExitTrapIsFunctionLocal, interp.Yes},
		{"ArithNameValueRecurses", s.ArithNameValueRecurses, interp.Yes},
		{"FatalErrorStatusIsOne", s.FatalErrorStatusIsOne, interp.Yes},
		{"EchoInterpretsEscapes", s.EchoInterpretsEscapes, interp.Yes},
		{"RegexQuotingMakesLiteral", s.RegexQuotingMakesLiteral, interp.No},
		{"ReadonlyReassignmentFatal", s.ReadonlyReassignmentFatal, interp.Yes},
		{"TraceAssignmentsSeparately", s.TraceAssignmentsSeparately, interp.No},
		{"TraceShowsItsOwnDisabling", s.TraceShowsItsOwnDisabling, interp.Yes},
		{"LocalInheritsTheExportAttribute", s.LocalInheritsTheExportAttribute, interp.No},
		{"StdinOptionNamesTheOperands", s.StdinOptionNamesTheOperands, interp.Yes},
		{"HangupIsAnOrderlyExit", s.HangupIsAnOrderlyExit, interp.Yes},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.axis, tc.got, tc.want)
		}
	}
	if got, want := s.UnterminatedBracket, interp.BracketBadPattern; got != want {
		t.Errorf("UnterminatedBracket = %v, want %v", got, want)
	}
	if got, want := s.ExitArgument, interp.ExitArgLenient; got != want {
		t.Errorf("ExitArgument = %v, want %v", got, want)
	}
}

// The invocation answers this preset gives that are not Answers.
func TestPlusSignedCommandStringNaming(t *testing.T) {
	if got := zsh.Semantics().PlusSignedCommandStringIsDollarZero; got != false {
		t.Errorf("PlusSignedCommandStringIsDollarZero = %v, want false", got)
	}
}

func TestDiagnosticAnswersTheInterpTestsRelyOn(t *testing.T) {
	d := zsh.Diagnostics()
	if got, want := d.TraceQuoting, interp.QuoteShell; got != want {
		t.Errorf("TraceQuoting = %v, want %v", got, want)
	}
	if got, want := d.TraceStyle, interp.TraceNameLine; got != want {
		t.Errorf("TraceStyle = %v, want %v", got, want)
	}
	if got, want := d.TraceForHeader, interp.TraceForAssign; got != want {
		t.Errorf("TraceForHeader = %v, want %v", got, want)
	}
	if got, want := d.Location, interp.LocationTightLine; got != want {
		t.Errorf("Location = %v, want %v", got, want)
	}
	if !d.NamesBuiltinInLocation {
		t.Error("NamesBuiltinInLocation = false, want the builtin named in the prefix")
	}
	// The two Report routes agree here; only one dialect splits them.
	if d.Report("s", 2, "m") != d.ForScript().Report("s", 2, "m") {
		t.Error("Report should not change between -c and a script")
	}
}

// TestBadPatternIsFatal is the composite the bracket policy stands for here:
// an unterminated bracket in a case pattern abandons the script with status 0,
// where the same pattern against the filesystem gives 1. Both are measured;
// neither is guessable from the other.
func TestBadPatternIsFatal(t *testing.T) {
	out, st := answersRun(t, `case "[" in [) echo hit;; *) echo miss;; esac; echo after`)
	if !strings.Contains(out, "bad pattern: [") {
		t.Errorf("got %q", out)
	}
	if strings.Contains(out, "after") || strings.Contains(out, "hit") || strings.Contains(out, "miss") {
		t.Errorf("the script should stop, got %q", out)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
	for _, src := range []string{`echo [a`, `echo a[`} {
		out, st := answersRun(t, src)
		if !strings.Contains(out, "bad pattern") {
			t.Errorf("%s: got %q", src, out)
		}
		if st != 1 {
			t.Errorf("%s: status = %d, want 1", src, st)
		}
	}
}

// TestEqualsExpansionComposite: `=cmd` becomes a path here, and a name that
// resolves to nothing is reported without a colon and abandons the script.
func TestEqualsExpansionComposite(t *testing.T) {
	if got, _ := answersRun(t, `echo =ls`); !strings.HasSuffix(strings.TrimSpace(got), "/ls") {
		t.Errorf("=ls should expand to a path, got %q", got)
	}
	out, st := answersRun(t, `echo =nosuchcommand_xyz; echo after`)
	if strings.Contains(out, "after") {
		t.Errorf("the script continued: %q", out)
	}
	if !strings.Contains(out, "nosuchcommand_xyz not found") {
		t.Errorf("got %q", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
}

// TestTraceShowsTheForAssignment: this preset's trace prints the assignment a
// `for` iteration made — with the name-and-line prefix, and without the
// trailing space it puts on an assignment that stands alone as a command.
func TestTraceShowsTheForAssignment(t *testing.T) {
	out, _ := answersRun(t, `set -x; for i in 1 2; do echo $i; done`)
	if !strings.Contains(out, "> i=1\n") || strings.Contains(out, "for i in") {
		t.Errorf("got %q, want an assignment and no header", out)
	}
}

// TestWordings runs the failures whose sentences are this shell's own.
func TestWordings(t *testing.T) {
	out, _ := answersRun(t, `readonly r=1; r=2`)
	if !strings.Contains(out, "read-only variable: r") {
		t.Errorf("readonly: got %q, want it to contain %q", out, "read-only variable: r")
	}
}

// A `test` diagnostic is written by the builtin under whichever of its two
// names was typed, so no wording may spell one of them itself. This dialect
// names the builtin in the location prefix instead, so its wordings carry no
// name at all.
func TestNoTestWordingSpellsItsOwnName(t *testing.T) {
	d := zsh.Diagnostics()
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

// TestALocalDoesNotCarryTheExportAttribute: a local shadowing an exported
// name hands a child nothing at all under that name here, where bash and dash
// hand it the local's value. The attribute is the local's to lose — the outer
// name is exported again the moment the function returns.
func TestALocalDoesNotCarryTheExportAttribute(t *testing.T) {
	out, _ := answersRun(t, `export FOO=bar; f() { local FOO=baz; /usr/bin/env | grep '^FOO=' || echo "(none)"; }; f; /usr/bin/env | grep '^FOO='`)
	if strings.Contains(out, "FOO=baz") {
		t.Errorf("got %q, want the child told nothing under the name", out)
	}
	if !strings.Contains(out, "(none)") || !strings.Contains(out, "FOO=bar") {
		t.Errorf("got %q, want nothing inside and the outer value after", out)
	}
	// A local declared without a value is set-and-empty here, and is not
	// exported either — so this is the same answer by the other road, where
	// bash and dash both tell the child something.
	out, _ = answersRun(t, `export FOO=bar; f() { local FOO; /usr/bin/env | grep '^FOO=' || echo "(none)"; }; f`)
	if !strings.Contains(out, "(none)") {
		t.Errorf("valueless: got %q, want the child told nothing", out)
	}
}

// TestAHangupEndsTheShellWithoutKillingIt is the composite this preset is
// alone in: an untrapped SIGHUP ends the script with 1 rather than with 128
// plus the number, and it runs the EXIT trap on the way out even though this
// shell does not run it for a signal that kills — which is the pair of facts
// that makes it an exit rather than a differently numbered death.
func TestAHangupEndsTheShellWithoutKillingIt(t *testing.T) {
	out, st := answersRun(t, "kill -HUP $$\necho after\n")
	if out != "" {
		t.Errorf("output %q, want the script to have stopped", out)
	}
	if st != 1 {
		t.Errorf("status %d, want 1", st)
	}
	out, st = answersRun(t, "trap 'echo bye' EXIT\nkill -HUP $$\necho after\n")
	if out != "bye\n" {
		t.Errorf("output %q, want the EXIT trap and nothing after", out)
	}
	if st != 1 {
		t.Errorf("status %d, want 1", st)
	}
	// The control, in the same preset: a signal that does kill leaves the
	// EXIT trap unrun and reports 128 plus the number.
	out, st = answersRun(t, "trap 'echo bye' EXIT\nkill -TERM $$\necho after\n")
	if out != "" {
		t.Errorf("output %q, want nothing — a death does not run the trap here", out)
	}
	if st != 128+15 {
		t.Errorf("status %d, want 143", st)
	}
}

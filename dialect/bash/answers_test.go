// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// The per-shell answers the interp tests used to assert inline: the interp
// package proves what each axis value does, and this file pins which value
// this preset gives, so a preset edit cannot silently flip one.

// answersRun parses and runs one snippet as this dialect, under `sh` and with
// a PATH, which is the only thing it adds to the shared builder.
//
// The dialect goes to the runner as well as to the parser, and both halves are
// load-bearing. The parser decides what the source *is*; the runner asks
// Runner.Dialect what a pattern means, whether arithmetic has floats, and what
// grammar nested input — a command substitution, an `eval`, a trap body, a
// sourced file — is parsed with. A runner built without one falls back to the
// core, so a test whose whole purpose is to assert this dialect's answer was
// asserting the core's: `[[ $k == a(b|c) ]]` parsed here and then did not
// match (#849, found closing #826). dialecttest.Preset.Runner is now the one
// place that field is set, for every helper in this package.
func answersRun(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Name: "sh", Env: []string{"PATH=/usr/bin:/bin"},
	}, src)
	if err != nil {
		return out + "unsupported: " + err.Error(), -1
	}
	return out, st
}

// TestTheHelperRunsUnderThisDialectAndNotTheCore guards the field answersRun
// sets, which nothing else in this package would miss.
//
// `${v^^}` is bash's and not the core's, and an `eval` reparses its argument
// with Runner.Dialect. Measured, bash 5.3.15:
//
//	$ bash -c 'eval "v=abc; echo \${v^^}"'
//	ABC
//
// Without the field the reparse happens under the core and answers
// `${v^^}: bad substitution` — a test in bash's own suite reporting on a
// shell that is not bash.
func TestTheHelperRunsUnderThisDialectAndNotTheCore(t *testing.T) {
	out, st := answersRun(t, `eval 'v=abc; echo ${v^^}'`)
	if strings.TrimSpace(out) != "ABC" || st != 0 {
		t.Errorf("answersRun = %q status %d, want ABC and 0: the runner was not told the dialect",
			out, st)
	}
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
		{"SubscriptCommaIsARange", s.SubscriptCommaIsARange, interp.No},
		{"ScalarSubscriptIsACharacter", s.ScalarSubscriptIsACharacter, interp.No},
		{"FatalErrorStatusIsOne", s.FatalErrorStatusIsOne, interp.Yes},
		{"RedirectErrorOnSpecialBuiltinFatal", s.RedirectErrorOnSpecialBuiltinFatal, interp.No},
		{"InteractiveMonitorNeedsATerminal", s.InteractiveMonitorNeedsATerminal, interp.Yes},
		{"InteractiveScriptAnnouncesJobs", s.InteractiveScriptAnnouncesJobs, interp.No},
		{"SubshellRunsOnAfterSignalingTheShell", s.SubshellRunsOnAfterSignalingTheShell, interp.Yes},
		{"UnsetReadonlyFatal", s.UnsetReadonlyFatal, interp.No},
		{"MultiDigitDuplicationTargetIsAnError", s.MultiDigitDuplicationTargetIsAnError, interp.No},
		{"EchoInterpretsEscapes", s.EchoInterpretsEscapes, interp.No},
		{"LengthOfSpecialIsCount", s.LengthOfSpecialIsCount, interp.Yes},
		{"SplitParamExpansion", s.SplitParamExpansion, interp.Yes},
		{"GlobNoMatchIsError", s.GlobNoMatchIsError, interp.No},
		{"ReadonlyReassignmentFatal", s.ReadonlyReassignmentFatal, interp.No},
		{"ShiftPastEndFatal", s.ShiftPastEndFatal, interp.No},
		{"TraceAssignmentsSeparately", s.TraceAssignmentsSeparately, interp.Yes},
		{"TraceShowsItsOwnDisabling", s.TraceShowsItsOwnDisabling, interp.Yes},
		{"LocalInheritsTheExportAttribute", s.LocalInheritsTheExportAttribute, interp.Yes},
		{"DeclarationAssignmentClearsTheExportAttribute", s.DeclarationAssignmentClearsTheExportAttribute, interp.No},
		{"StdinOptionNamesTheOperands", s.StdinOptionNamesTheOperands, interp.No},
		{"HangupIsAnOrderlyExit", s.HangupIsAnOrderlyExit, interp.No},
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
	if got, want := s.SubshellJobTable, interp.SubshellJobsKeptOutsideACompound; got != want {
		t.Errorf("SubshellJobTable = %v, want %v", got, want)
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

// A duplication target wider than one digit is a number like any other here:
// the descriptor is looked at, the failure is the descriptor's, and the script
// runs on. One shell in the panel refuses the word instead, and asserting this
// side as behavior is what keeps the preset from drifting into that answer.
func TestAWideDuplicationTargetIsReadAsANumber(t *testing.T) {
	out, st := answersRun(t, "echo hi >&10\necho after\n")
	if !strings.Contains(out, "10: Bad file descriptor") {
		t.Errorf("out %q, want the descriptor's own failure", out)
	}
	if !strings.Contains(out, "after") || st != 0 {
		t.Errorf("out %q status %d, want the script to have carried on", out, st)
	}
}

// `$[expr]`, the older spelling of `$((expr))`, which this shell still takes.
//
// Measured 2026-09-06 on bash 5.3.15 and on the 3.2.57 macOS ships: both give
// 2 for `echo $[1+1]`, and both give the arithmetic diagnostics word for word
// where the expression is bad. The manual has called it deprecated for years,
// which is a fact about the manual and not about either parser (#900).
func TestTheOlderArithmeticSpelling(t *testing.T) {
	out, st := answersRun(t, `x=5; a=(7 8 9); echo "[$[1+1]][$[x*2]][$[a[1]+1]][$[2**10]]"`)
	if want := "[2][10][9][1024]\n"; out != want || st != 0 {
		t.Errorf("got %q at %d, want %q at 0", out, st, want)
	}
	// The two spellings are the same expression to everything downstream, so
	// they answer with the same value where they answer at all. A bad
	// expression is refused in the same words too, which is graded rather
	// than asserted here: `arith/a-dollar-bracket-refuses-like-the-other`
	// puts both wordings beside the panel's.
	if out, st := answersRun(t, `echo "[$[2**10]][$((2**10))][$[]][$(( ))]"`); out != "[1024][1024][0][0]\n" || st != 0 {
		t.Errorf("got %q at %d, want the two spellings to agree", out, st)
	}
}

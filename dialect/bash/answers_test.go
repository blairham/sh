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
		{"LastBackgroundPidIsUnsetBeforeAnyJob", s.LastBackgroundPidIsUnsetBeforeAnyJob, interp.Yes},
		{"LastBackgroundPidIsZeroBeforeAnyJob", s.LastBackgroundPidIsZeroBeforeAnyJob, interp.No},
		{"ExitTrapIsFunctionLocal", s.ExitTrapIsFunctionLocal, interp.No},
		{"ArithNameValueRecurses", s.ArithNameValueRecurses, interp.Yes},
		{"ArithRecursedNameMustBeSet", s.ArithRecursedNameMustBeSet, interp.No},
		{"ArithInvalidOctalDigitIsError", s.ArithInvalidOctalDigitIsError, interp.Yes},
		{"IntegerAssignmentReadsALeadingZeroAsDecimal", s.IntegerAssignmentReadsALeadingZeroAsDecimal, interp.No},
		{"ArrayBaseIsZero", s.ArrayBaseIsZero, interp.Yes},
		{"ArrayLiteralSubscriptIsAKey", s.ArrayLiteralSubscriptIsAKey, interp.No},
		{"SubscriptCommaIsARange", s.SubscriptCommaIsARange, interp.No},
		{"SubscriptIsAQuotingContext", s.SubscriptIsAQuotingContext, interp.Yes},
		{"ScalarSubscriptIsACharacter", s.ScalarSubscriptIsACharacter, interp.No},
		{"FatalErrorStatusIsOne", s.FatalErrorStatusIsOne, interp.Yes},
		{"RedirectErrorOnSpecialBuiltinFatal", s.RedirectErrorOnSpecialBuiltinFatal, interp.No},
		{"DuplicationTargetErrorOnABuiltinIsFatal", s.DuplicationTargetErrorOnABuiltinIsFatal, interp.No},
		{"InteractiveMonitorNeedsATerminal", s.InteractiveMonitorNeedsATerminal, interp.Yes},
		{"InteractiveScriptAnnouncesJobs", s.InteractiveScriptAnnouncesJobs, interp.No},
		{"SubshellRunsOnAfterSignalingTheShell", s.SubshellRunsOnAfterSignalingTheShell, interp.Yes},
		{"UnsetReadonlyFatal", s.UnsetReadonlyFatal, interp.No},
		{"MultiDigitDuplicationTargetIsAnError", s.MultiDigitDuplicationTargetIsAnError, interp.No},
		{"EchoInterpretsEscapes", s.EchoInterpretsEscapes, interp.No},
		{"LengthOfSpecialIsCount", s.LengthOfSpecialIsCount, interp.Yes},
		{"SplitParamExpansion", s.SplitParamExpansion, interp.Yes},
		{"UnquotedListJoinsOnIFS", s.UnquotedListJoinsOnIFS, interp.Yes},
		// The other join, and the opposite answer: an unquoted `@` list
		// reaching a context that keeps no fields is rejoined on a hard
		// space here — `IFS=-; a=(x y z); v=${a[@]}` is `x y z`, against
		// `x-y-z` in zsh. The `*` spelling is core and does not ask this.
		{"UnsplitAtListJoinsOnIFS", s.UnsplitAtListJoinsOnIFS, interp.No},
		{"TrailingSeparatorEndsAField", s.TrailingSeparatorEndsAField, interp.No},
		{"GlobNoMatchIsError", s.GlobNoMatchIsError, interp.No},
		{"ReadonlyReassignmentFatal", s.ReadonlyReassignmentFatal, interp.No},
		{"AssignThroughExpansionMayNameAPositional", s.AssignThroughExpansionMayNameAPositional, interp.No},
		{"ShiftPastEndFatal", s.ShiftPastEndFatal, interp.No},
		{"TraceAssignmentsSeparately", s.TraceAssignmentsSeparately, interp.Yes},
		{"TraceShowsItsOwnDisabling", s.TraceShowsItsOwnDisabling, interp.Yes},
		{"LocalInheritsTheExportAttribute", s.LocalInheritsTheExportAttribute, interp.Yes},
		{"DeclarationAssignmentClearsTheExportAttribute", s.DeclarationAssignmentClearsTheExportAttribute, interp.No},
		{"UnsetSubscriptOnAScalarIsAnError", s.UnsetSubscriptOnAScalarIsAnError, interp.Yes},
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
	if got, want := s.BackgroundJobInput, interp.BackgroundJobInputEmpty; got != want {
		t.Errorf("BackgroundJobInput = %v, want %v", got, want)
	}
	if got, want := s.StatusArgument, interp.StatusArgNumeric; got != want {
		t.Errorf("StatusArgument = %v, want %v", got, want)
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

// The three array axes #1571 and #1572 opened, pinned here as behavior so a
// preset edit cannot silently flip one — which is what this file is for, and
// what nothing else would have caught: the interp suites set the axes
// themselves and prove what each *value* does, never which value this preset
// gives.
//
// `a+=x` over a name holding an array joins the first element and leaves the
// rest standing. Measured 2026-09-08 on 5.3.15, on the same binary under
// argv[0] of `sh`, and on the 3.2.57 macOS ships — all three
// `declare -a a=([0]="1x" [1]="2")`, two elements, where zsh has three.
func TestAScalarAppendedToAnArrayJoinsTheFirstElement(t *testing.T) {
	out, st := answersRun(t, `a=(1 2); a+=x; printf '[%s]' "${a[@]}"; echo " n=${#a[@]}"`)
	if want := "[1x][2] n=2\n"; out != want || st != 0 {
		t.Errorf("got %q at %d, want %q at 0", out, st, want)
	}
}

// Both array letters given to a name already holding a scalar promote that
// value: `b=1; typeset -a b` lists `declare -a b=([0]="1")` and
// `b=1; typeset -A b` lists `declare -A b=([0]="1" )`, measured 2026-09-08.
// 3.2.57 answers the array letter the same way and has no `-A` at all.
//
// The *listing* is what is asserted and the count is not enough on its own:
// ksh93 leaves the name a scalar, and a scalar there reads back through
// `${b[@]}` and `${#b[@]}` exactly as this shell's promotion does — so a probe
// through the elements alone would pass under that shell's answer too.
func TestAnArrayLetterOverAScalarPromotesTheValue(t *testing.T) {
	for _, tc := range []struct{ name, src, list string }{
		{"the array letter", `typeset -a b`, `declare -a b=([0]="1")`},
		{"the table letter", `typeset -A b`, `declare -A b=([0]="1" )`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, `b=1; `+tc.src+`; typeset -p b; echo "n=${#b[@]}"`)
			if !strings.Contains(out, tc.list) || !strings.Contains(out, "n=1") || st != 0 {
				t.Errorf("got %q at %d, want %q and a count of 1 at 0", out, st, tc.list)
			}
		})
	}
}

// TestAnAssignmentThroughAnExpansionCannotNameAListOrAPositional is #1541.
//
// Measured 2026-09-11 on 5.3.15, on the same binary under argv[0] of `sh` and
// on 3.2.57, all three alike. The sigil is written back — `$@` and `$1`,
// where dash names the bare letter — and the positional is refused with the
// list rather than assigned.
//
// The line is given up and the shell is not: this is the one column that
// abandons a failed word and carries on, so the row after it still runs.
func TestAnAssignmentThroughAnExpansionCannotNameAListOrAPositional(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`set --; printf "<%s>" ${@:=abc}`, "$@: cannot assign in this way"},
		{`set --; printf "<%s>" ${*:=abc}`, "$*: cannot assign in this way"},
		{`set --; printf "<%s>" ${1:=abc}`, "$1: cannot assign in this way"},
	} {
		out, st := answersRun(t, tc.src+"\necho AFTER")
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s = %q, want %q in it", tc.src, out, tc.want)
		}
		if !strings.Contains(out, "AFTER") || st != 0 {
			t.Errorf("%s = %q (status %d), want the next line to run at 0", tc.src, out, st)
		}
	}
	// The control, unanimous across the panel: the operator does not fire on
	// a parameter that is there, so nothing is refused.
	out, st := answersRun(t, `set -- p; printf "<%s>" ${@:=abc}`)
	if out != "<p>" || st != 0 {
		t.Errorf("a parameter that is there = %q (status %d), want <p> at 0", out, st)
	}
}

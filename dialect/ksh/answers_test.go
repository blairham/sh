// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
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
		{"ProcessSubstitutionBodyReadsTheShellsInput", s.ProcessSubstitutionBodyReadsTheShellsInput, interp.No},
		{"UnsetPositionalIsAllowed", s.UnsetPositionalIsAllowed, interp.Yes},
		{"LastBackgroundPidIsUnsetBeforeAnyJob", s.LastBackgroundPidIsUnsetBeforeAnyJob, interp.No},
		{"LastBackgroundPidIsZeroBeforeAnyJob", s.LastBackgroundPidIsZeroBeforeAnyJob, interp.No},
		{"ExitTrapIsFunctionLocal", s.ExitTrapIsFunctionLocal, interp.No},
		{"ArithNameValueRecurses", s.ArithNameValueRecurses, interp.Yes},
		{"ArithRecursedNameMustBeSet", s.ArithRecursedNameMustBeSet, interp.Yes},
		{"ArithInvalidOctalDigitIsError", s.ArithInvalidOctalDigitIsError, interp.No},
		{"IntegerAssignmentReadsALeadingZeroAsDecimal", s.IntegerAssignmentReadsALeadingZeroAsDecimal, interp.Yes},
		{"ArrayBaseIsZero", s.ArrayBaseIsZero, interp.Yes},
		{"ArrayLiteralSubscriptIsAKey", s.ArrayLiteralSubscriptIsAKey, interp.Yes},
		{"SubscriptCommaIsARange", s.SubscriptCommaIsARange, interp.No},
		{"SubscriptIsAQuotingContext", s.SubscriptIsAQuotingContext, interp.Yes},
		{"ScalarSubscriptIsACharacter", s.ScalarSubscriptIsACharacter, interp.No},
		// Alone in the panel on the slice, and on the same side as bash on
		// the length: `${h[@]:0:1}` is the whole value here and one
		// character everywhere else, while `${#h[@]}` is 1 here, 1 in both
		// bashes and 3 only in zsh. Two axes because no one answer gives
		// both of those partitions.
		{"WholeSubscriptOnAScalarMeasuresIt", s.WholeSubscriptOnAScalarMeasuresIt, interp.No},
		{"WholeSubscriptOnAScalarSlicesIt", s.WholeSubscriptOnAScalarSlicesIt, interp.No},
		{"FatalErrorStatusIsOne", s.FatalErrorStatusIsOne, interp.Yes},
		{"RedirectErrorOnSpecialBuiltinFatal", s.RedirectErrorOnSpecialBuiltinFatal, interp.Yes},
		{"DuplicationTargetErrorOnABuiltinIsFatal", s.DuplicationTargetErrorOnABuiltinIsFatal, interp.No},
		{"InteractiveMonitorNeedsATerminal", s.InteractiveMonitorNeedsATerminal, interp.No},
		{"InteractiveScriptAnnouncesJobs", s.InteractiveScriptAnnouncesJobs, interp.Yes},
		{"SubshellRunsOnAfterSignalingTheShell", s.SubshellRunsOnAfterSignalingTheShell, interp.No},
		{"UnsetReadonlyFatal", s.UnsetReadonlyFatal, interp.No},
		{"MultiDigitDuplicationTargetIsAnError", s.MultiDigitDuplicationTargetIsAnError, interp.No},
		{"SplitParamExpansion", s.SplitParamExpansion, interp.Yes},
		{"UnquotedListJoinsOnIFS", s.UnquotedListJoinsOnIFS, interp.No},
		// bash's answer on this one against its own on the axis above: the
		// two partition the panel differently, which is why neither can
		// stand in for the other.
		{"UnsplitAtListJoinsOnIFS", s.UnsplitAtListJoinsOnIFS, interp.No},
		{"TrailingSeparatorEndsAField", s.TrailingSeparatorEndsAField, interp.No},
		{"GlobNoMatchIsError", s.GlobNoMatchIsError, interp.No},
		// The one column that reads a value's backslash as data, so the
		// metacharacter behind it stays live (#1367).
		{"ValueBackslashQuotesWhatFollows", s.ValueBackslashQuotesWhatFollows, interp.No},
		{"PositionalListWithNoneIsSet", s.PositionalListWithNoneIsSet, interp.No},
		{"PrefixToAFrozenNameIsCheckedFirst", s.PrefixToAFrozenNameIsCheckedFirst, interp.No},
		{"ReadonlyReassignmentFatal", s.ReadonlyReassignmentFatal, interp.Yes},
		{"EmptyParamSubscriptIsAnError", s.EmptyParamSubscriptIsAnError, interp.No},
		{"EmptyAssociativeKeyIsAnError", s.EmptyAssociativeKeyIsAnError, interp.No},
		{"EmptyAssociativeKeyIsReportedWhenRead", s.EmptyAssociativeKeyIsReportedWhenRead, interp.No},
		{"AssignThroughExpansionMayNameAPositional", s.AssignThroughExpansionMayNameAPositional, interp.No},
		{"ShiftPastEndFatal", s.ShiftPastEndFatal, interp.Yes},
		{"TraceAssignmentsSeparately", s.TraceAssignmentsSeparately, interp.Yes},
		{"TraceShowsItsOwnDisabling", s.TraceShowsItsOwnDisabling, interp.No},
		{"LocalInheritsTheExportAttribute", s.LocalInheritsTheExportAttribute, interp.No},
		{"DeclarationAssignmentClearsTheExportAttribute", s.DeclarationAssignmentClearsTheExportAttribute, interp.Yes},
		{"UnsetSubscriptOnAScalarIsAnError", s.UnsetSubscriptOnAScalarIsAnError, interp.No},
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
	if got, want := s.BackgroundJobInput, interp.BackgroundJobInputEmptyUnlessClosed; got != want {
		t.Errorf("BackgroundJobInput = %v, want %v", got, want)
	}
	if got, want := s.StatusArgument, interp.StatusArgLeadingDigits; got != want {
		t.Errorf("StatusArgument = %v, want %v", got, want)
	}
	if got, want := s.ReadTrailingEscapedSeparator, interp.ReadTrailingEscapedSeparatorTrimmed; got != want {
		t.Errorf("ReadTrailingEscapedSeparator = %v, want %v", got, want)
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
	if got, want := d.TraceCaseHeader, interp.TraceCaseNone; got != want {
		t.Errorf("TraceCaseHeader = %v, want %v", got, want)
	}
	if got, want := d.TraceCondition, interp.TraceCondPrimary; got != want {
		t.Errorf("TraceCondition = %v, want %v", got, want)
	}
	if got, want := d.TraceConditionQuoting, interp.QuoteDollar; got != want {
		t.Errorf("TraceConditionQuoting = %v, want %v", got, want)
	}
	// The text between the parentheses and nothing added, at both sites.
	if got, want := d.TraceArithCommand, interp.TraceArithTight; got != want {
		t.Errorf("TraceArithCommand = %v, want %v", got, want)
	}
	if got, want := d.TraceArithForPart, interp.TraceArithTight; got != want {
		t.Errorf("TraceArithForPart = %v, want %v", got, want)
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

// `$[expr]` is not arithmetic here, which is the half of #900 that says the
// spelling is a grammar flag rather than something the core has.
//
// Measured 2026-09-06: ksh93u+ leaves the text alone — the `$` is literal and
// the brackets are a pattern, so with nothing on the filesystem to match, the
// word stands as written. bash and zsh both read it as arithmetic.
func TestTheOlderArithmeticSpellingIsNotRead(t *testing.T) {
	out, st := answersRun(t, `x=5; echo "[$[1+1]][$[x*2]]"`)
	if want := "[$[1+1]][$[x*2]]\n"; out != want || st != 0 {
		t.Errorf("got %q at %d, want %q at 0", out, st, want)
	}
}

// The three array axes #1571 and #1572 opened, pinned here as behavior so a
// preset edit cannot silently flip one — and this shell is the reason two of
// them are separate axes at all.
//
// `a+=x` over a name holding an array joins the first element and leaves the
// rest standing: `typeset -a a=(1x 2)`, two elements, measured 2026-09-08 on
// ksh93u+. bash agrees; zsh adds a third element instead.
func TestAScalarAppendedToAnArrayJoinsTheFirstElement(t *testing.T) {
	out, st := answersRun(t, `a=(1 2); a+=x; printf '[%s]' "${a[@]}"; echo " n=${#a[@]}"`)
	if want := "[1x][2] n=2\n"; out != want || st != 0 {
		t.Errorf("got %q at %d, want %q at 0", out, st, want)
	}
}

// The two array letters over a name already holding a scalar answer
// *differently here*, which is the whole of why they are two fields rather
// than one: `b=1; typeset -a b` converts nothing and records nothing, so
// `typeset -p b` still says `b=1`, while `b=1; typeset -A b` promotes the
// value under the key `0` and lists `typeset -A b=([0]=1)`. Measured
// 2026-09-08. bash promotes under both letters and zsh discards under both.
//
// The listing is the only thing that separates the array letter's answer from
// bash's: this shell lets a scalar be subscripted, so `${b[0]}` and
// `${#b[@]}` read the same either way. A bare `typeset -a` afterwards — which
// lists every name carrying the attribute — prints nothing, which is the
// second probe that says the declaration recorded nothing.
func TestTheTwoArrayLettersOverAScalarAnswerDifferently(t *testing.T) {
	for _, tc := range []struct{ name, src, list string }{
		{"the array letter converts nothing", `typeset -a b`, "b=1"},
		{"the table letter promotes", `typeset -A b`, `typeset -A b=([0]=1)`},
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
// Measured 2026-09-11. This shell names neither the parameter nor the
// operator: the whole *word* is blamed, which is what the rows with text and
// quotes around the expansion are for — `x${@:=abc}y` and `"${@:=abc}"` are
// each reported entire.
func TestAnAssignmentThroughAnExpansionCannotNameAListOrAPositional(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`set --; printf "<%s>" ${@:=abc}`, "${@:=abc}: bad substitution"},
		{`set --; printf "<%s>" ${*:=abc}`, "${*:=abc}: bad substitution"},
		{`set --; printf "<%s>" ${1:=abc}`, "${1:=abc}: bad substitution"},
		{`set --; printf "<%s>" x${@:=abc}y`, "x${@:=abc}y: bad substitution"},
		{`set --; printf "<%s>" "${@:=abc}"`, `"${@:=abc}": bad substitution`},
	} {
		out, st := answersRun(t, tc.src+"\necho AFTER")
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s = %q, want %q in it", tc.src, out, tc.want)
		}
		if strings.Contains(out, "AFTER") || st != 1 {
			t.Errorf("%s = %q (status %d), want the shell ended at 1", tc.src, out, st)
		}
	}
	out, st := answersRun(t, `set -- p; printf "<%s>" ${@:=abc}`)
	if out != "<p>" || st != 0 {
		t.Errorf("a parameter that is there = %q (status %d), want <p> at 0", out, st)
	}
}

// TestAnEmptyParameterSubscriptIsTheEmptyExpression is #1763, and this is the
// one column that reads it.
//
// The brackets hold an expression that happens to be empty, which is zero — so
// the subscript names element zero rather than failing. Measured 2026-09-11 on
// 93u+: an indexed array answers its first element, a scalar answers itself,
// and a name nothing declared answers nothing at all. The other five columns
// refuse the expansion.
//
// The scalar row is what separates this from "the number zero": a policy
// worded that way would have answered `0` where this shell answers `hi`.
func TestAnEmptyParameterSubscriptIsTheEmptyExpression(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`a=(5 6 7); echo "[${a[]}]"`, "[5]"},
		{`s=hi; echo "[${s[]}]"`, "[hi]"},
		{`echo "[${nodecl[]}]"`, "[]"},
		{`a=(5 6 7); echo "[${#a[]}]"`, "[1]"},
		{`a=(5 6 7); echo "[${a[]:-d}]"`, "[5]"},
	} {
		out, st := answersRun(t, tc.src)
		if strings.TrimSpace(out) != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %s at 0", tc.src, out, st, tc.want)
		}
	}
}

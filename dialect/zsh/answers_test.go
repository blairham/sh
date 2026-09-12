// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// The per-shell answers the interp tests used to assert inline: the interp
// package proves what each axis value does, and this file pins which value
// this preset gives — plus the few composites that are this shell's alone.

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
// A directory of its own, too. A snippet that globs needs files to match,
// and without this the runner works where the package's source is — so a row
// that made one left it in the tree, and `dialect/zsh/v5` was committed
// before anyone noticed (#1221).
func answersRun(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Name: "sh", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
	}, src)
	if err != nil {
		return out + "unsupported: " + err.Error(), -1
	}
	return out, st
}

// TestTheHelperRunsUnderThisDialectAndNotTheCore guards the field answersRun
// sets. The pattern-matching half of the same claim is in
// condalternation_test.go, which is where #849 was found; this is the nested
// parse, which no dialect's suite covered.
//
// `repeat` is this shell's loop and not the core's, and an `eval` reparses
// its argument with Runner.Dialect. Measured, zsh 5.9.2:
//
//	$ zsh -c 'eval "repeat 2 echo hi"'
//	hi
//	hi
//
// Without the field the reparse happens under the core, where `repeat` is
// not a keyword and there is no such command.
func TestTheHelperRunsUnderThisDialectAndNotTheCore(t *testing.T) {
	out, st := answersRun(t, `eval 'repeat 2 echo hi'`)
	if strings.TrimSpace(out) != "hi\nhi" || st != 0 {
		t.Errorf("answersRun = %q status %d, want two lines of hi: the runner was not told the dialect",
			out, st)
	}
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
		{"ProcessSubstitutionBodyReadsTheShellsInput", s.ProcessSubstitutionBodyReadsTheShellsInput, interp.Yes},
		{"UnsetPositionalIsAllowed", s.UnsetPositionalIsAllowed, interp.No},
		{"LastBackgroundPidIsUnsetBeforeAnyJob", s.LastBackgroundPidIsUnsetBeforeAnyJob, interp.No},
		{"LastBackgroundPidIsZeroBeforeAnyJob", s.LastBackgroundPidIsZeroBeforeAnyJob, interp.Yes},
		{"ExitTrapIsFunctionLocal", s.ExitTrapIsFunctionLocal, interp.Yes},
		{"ArithNameValueRecurses", s.ArithNameValueRecurses, interp.Yes},
		{"ArithRecursedNameMustBeSet", s.ArithRecursedNameMustBeSet, interp.No},
		{"FatalErrorStatusIsOne", s.FatalErrorStatusIsOne, interp.Yes},
		{"RedirectErrorOnSpecialBuiltinFatal", s.RedirectErrorOnSpecialBuiltinFatal, interp.No},
		{"DuplicationTargetErrorOnABuiltinIsFatal", s.DuplicationTargetErrorOnABuiltinIsFatal, interp.Yes},
		{"InteractiveMonitorNeedsATerminal", s.InteractiveMonitorNeedsATerminal, interp.Yes},
		{"InteractiveScriptAnnouncesJobs", s.InteractiveScriptAnnouncesJobs, interp.Yes},
		{"SubshellRunsOnAfterSignalingTheShell", s.SubshellRunsOnAfterSignalingTheShell, interp.Yes},
		{"UnsetReadonlyFatal", s.UnsetReadonlyFatal, interp.Yes},
		{"MultiDigitDuplicationTargetIsAnError", s.MultiDigitDuplicationTargetIsAnError, interp.No},
		{"EchoInterpretsEscapes", s.EchoInterpretsEscapes, interp.Yes},
		{"RegexQuotingMakesLiteral", s.RegexQuotingMakesLiteral, interp.No},
		{"ReadonlyReassignmentFatal", s.ReadonlyReassignmentFatal, interp.Yes},
		{"EmptyParamSubscriptIsAnError", s.EmptyParamSubscriptIsAnError, interp.Yes},
		{"EmptyAssociativeKeyIsAnError", s.EmptyAssociativeKeyIsAnError, interp.No},
		{"EmptyAssociativeKeyIsReportedWhenRead", s.EmptyAssociativeKeyIsReportedWhenRead, interp.No},
		{"AssignThroughExpansionMayNameAPositional", s.AssignThroughExpansionMayNameAPositional, interp.Yes},
		{"TraceAssignmentsSeparately", s.TraceAssignmentsSeparately, interp.No},
		{"TraceShowsItsOwnDisabling", s.TraceShowsItsOwnDisabling, interp.Yes},
		{"LocalInheritsTheExportAttribute", s.LocalInheritsTheExportAttribute, interp.No},
		{"DeclarationAssignmentClearsTheExportAttribute", s.DeclarationAssignmentClearsTheExportAttribute, interp.No},
		{"UnsetSubscriptOnAScalarIsAnError", s.UnsetSubscriptOnAScalarIsAnError, interp.No},
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
	if got, want := s.UnknownCharacterClass, interp.UnknownClassIsInert; got != want {
		t.Errorf("UnknownCharacterClass = %v, want %v", got, want)
	}
	if got, want := s.BackgroundJobInput, interp.BackgroundJobInputIsTheShells; got != want {
		t.Errorf("BackgroundJobInput = %v, want %v", got, want)
	}
	if got, want := s.StatusArgument, interp.StatusArgArithmetic; got != want {
		t.Errorf("StatusArgument = %v, want %v", got, want)
	}
	if got, want := s.SubshellJobTable, interp.SubshellJobsCleared; got != want {
		t.Errorf("SubshellJobTable = %v, want %v", got, want)
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
	if got, want := d.TraceCaseHeader, interp.TraceCaseArm; got != want {
		t.Errorf("TraceCaseHeader = %v, want %v", got, want)
	}
	if got, want := d.TraceCondition, interp.TraceCondWhole; got != want {
		t.Errorf("TraceCondition = %v, want %v", got, want)
	}
	if got, want := d.TraceConditionQuoting, interp.QuoteShell; got != want {
		t.Errorf("TraceConditionQuoting = %v, want %v", got, want)
	}
	// The two arithmetic sites disagree in this dialect alone: a `(( ))`
	// command keeps its parentheses and a `for ((;;))` part does not.
	if got, want := d.TraceArithCommand, interp.TraceArithSpaced; got != want {
		t.Errorf("TraceArithCommand = %v, want %v", got, want)
	}
	if got, want := d.TraceArithForPart, interp.TraceArithBare; got != want {
		t.Errorf("TraceArithForPart = %v, want %v", got, want)
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

// `$[expr]`, this shell's other arithmetic spelling and the older one.
//
// Measured 2026-09-06: `echo $[1+1]` is 2 here and in bash 5.3, bash 3.2 and
// bash as `sh`, and the same text is the literal `$[1+1]` in ksh93 and dash.
// Reading it as a glob is what made the failure `no matches found`, which
// points a person at globbing rather than at arithmetic (#900).
func TestTheOtherArithmeticSpelling(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a sum", `echo $[1+1]`, "2\n"},
		{"a name", `x=5; echo $[x*2]`, "10\n"},
		{"in double quotes", `echo "[$[2+3]]"`, "[5]\n"},
		{"joined to a word", `echo a$[4+4]b`, "a8b\n"},
		{"a subscript inside", `a=(7 8 9); echo $[a[1]+1]`, "8\n"},
		{"nested in itself", `echo $[$[2+2]*2]`, "8\n"},
		{"in an operand", `echo ${p:-$[3*3]}`, "9\n"},
		{"single quotes make it text", `echo '$[1+1]'`, "$[1+1]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// The three array axes #1571 and #1572 opened, pinned here as behavior so a
// preset edit cannot silently flip one — and this shell is the odd column in
// all three.
//
// `a+=x` over a name holding an array adds a *new element* after the last
// rather than joining the first: `typeset -a a=( 1 2 x )`, three elements,
// measured 2026-09-08 on 5.9.2, where bash and ksh93 have two. Note this is
// not the same question as `a+=(x)`, which every shell with arrays agrees
// about (#1502).
func TestAScalarAppendedToAnArrayAddsANewElement(t *testing.T) {
	out, st := answersRun(t, `a=(1 2); a+=x; printf '[%s]' "${a[@]}"; echo " n=${#a[@]}"`)
	if want := "[1][2][x] n=3\n"; out != want || st != 0 {
		t.Errorf("got %q at %d, want %q at 0", out, st, want)
	}
}

// Both array letters given to a name already holding a scalar throw that value
// away and leave the name an empty compound: `b=1; typeset -a b` is
// `typeset -a b=(  )` and `b=1; typeset -A b` is `typeset -A b=( )`, both with
// `${#b[@]}` of 0 and `$b` empty. Measured 2026-09-08.
//
// This is the one column that loses the script's own value, and it was the
// answer this implementation gave every dialect — matching this shell by
// accident and losing the value in the other two at status 0 (#1572). The
// count is what is asserted, because it is the count that separates this
// answer from the other two.
func TestAnArrayLetterOverAScalarDiscardsTheValue(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"the array letter", `typeset -a b`},
		{"the table letter", `typeset -A b`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, `b=1; `+tc.src+`; printf '[%s]' "${b[@]}"; echo " n=${#b[@]} v=[$b]"`)
			if want := "[] n=0 v=[]\n"; out != want || st != 0 {
				t.Errorf("got %q at %d, want %q at 0", out, st, want)
			}
		})
	}
}

// TestAnAssignmentThroughAnExpansionNamesAPositionalAndNotAList is #1541.
//
// The one column in the panel that *assigns* through the conditional
// operator: measured 2026-09-11 on 5.9.2, `set --; printf "<%s>" ${1:=abc}`
// is `abc` at status 0 and leaves `$1` holding it, where the other five
// refuse. `@` and `*` are refused here too, which is what makes it the
// positional alone rather than the whole family.
//
// The assertion on the assigning row is on `$1` afterwards: substituting the
// word and storing nothing looks identical on the line itself, and that is
// the wrong answer #1541 is about.
func TestAnAssignmentThroughAnExpansionNamesAPositionalAndNotAList(t *testing.T) {
	out, st := answersRun(t, `set --; printf "<%s>" ${1:=abc}; printf "|one=%s" "$1"`)
	if out != "<abc>|one=abc" || st != 0 {
		t.Errorf("the positional = %q (status %d), want it stored at 0", out, st)
	}
	for _, tc := range []struct{ src, want string }{
		{`set --; printf "<%s>" ${@:=abc}`, "not an identifier: @"},
		{`set --; printf "<%s>" ${*:=abc}`, "not an identifier: *"},
	} {
		out, st := answersRun(t, tc.src+"\necho AFTER")
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s = %q, want %q in it", tc.src, out, tc.want)
		}
		if strings.Contains(out, "AFTER") || st != 1 {
			t.Errorf("%s = %q (status %d), want the shell ended at 1", tc.src, out, st)
		}
	}
}

// TestAnEmptyParameterSubscriptIsInvalid is #1763.
//
// `invalid subscript` — the subscript machinery's own sentence, with no name in
// it and no bad-substitution wording around it — and the expansion is refused.
// Measured 2026-09-11 on 5.9.2.
//
// It refuses **whether or not the name exists**, which is the half that makes
// this a different question from the arithmetic site: there the same shell is a
// silent zero on a name nothing declared and `invalid subscript` on one that
// is set, and here it is the refusal either way.
func TestAnEmptyParameterSubscriptIsInvalid(t *testing.T) {
	for _, src := range []string{
		`a=(5 6 7); echo "[${a[]}]"`,
		`s=hi; echo "[${s[]}]"`,
		`echo "[${nodecl[]}]"`,
		`a=(5 6 7); echo "[${#a[]}]"`,
		`a=(5 6 7); echo "[${a[]:-d}]"`,
	} {
		out, st := answersRun(t, src+"\necho AFTER")
		if !strings.Contains(out, "invalid subscript") || strings.Contains(out, "bad substitution") {
			t.Errorf("%s = %q, want `invalid subscript` and no substitution wording", src, out)
		}
		if strings.Contains(out, "AFTER") || st == 0 {
			t.Errorf("%s = %q (status %d), want the shell ended", src, out, st)
		}
	}
}

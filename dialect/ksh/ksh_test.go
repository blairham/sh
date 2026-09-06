// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"bytes"
	"context"
	"strings"
	"syscall"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The measured answers for ksh93 live here, in ksh93's own package, so that adding
// or correcting a shell never touches the substrate.

func parses(t *testing.T, src string) bool {
	t.Helper()
	_, err := syntax.Parse(src, ksh.Dialect())
	return err == nil
}

func TestGrammar(t *testing.T) {
	// Expands aliases in a script, with no option to turn on.
	if got, want := ksh.Dialect().ExpandAliases, syntax.AliasOnEveryRoute; got != want {
		t.Errorf("ExpandAliases = %v, want %v", got, want)
	}
	// And a body's newlines are lines of the program: $LINENO after a
	// two-line body reads 6 against a physical 5.
	if !ksh.Dialect().AliasBodyCountsLines {
		t.Error("AliasBodyCountsLines = false, want true")
	}
	for _, tc := range []struct {
		src  string
		want bool
	}{
		{`case a in a) echo x;;& esac`, false},
		{`echo ${x^^}`, false},
		{`echo ${!x}`, true},
		{`function f() { echo x; }`, false},
		{`function f { echo x; }`, true},
		{`a=(x y)`, true},
	} {
		if got := parses(t, tc.src); got != tc.want {
			t.Errorf("%q: parses = %v, want %v", tc.src, got, tc.want)
		}
	}
}

func TestSemantics(t *testing.T) {
	// The same answer bash and dash give.
	if got, want := ksh.Semantics().ExitInTrapReportsEarlierStatus, interp.Yes; got != want {
		t.Errorf("ExitInTrapReportsEarlierStatus = %v, want %v", got, want)
	}
	s := ksh.Semantics()
	for _, tc := range []struct {
		axis string
		got  interp.Answer
		want interp.Answer
	}{
		{"SetFTurnsOffGlobbing", s.SetFTurnsOffGlobbing, interp.Yes},
		// The file comparisons side with bash on a missing file and with
		// zsh on a non-numeric -t operand.
		{"MissingFileIsOlder", s.MissingFileIsOlder, interp.Yes},
		{"TerminalTestRequiresANumber", s.TerminalTestRequiresANumber, interp.No},
		// ksh93 has no name for the pipeline status, so neither axis
		// arises; the scalar view of an array does.
		{"AssignmentUpdatesPipelineStatus", s.AssignmentUpdatesPipelineStatus, interp.Unspecified},
		{"ArrayScalarIsTheWholeArray", s.ArrayScalarIsTheWholeArray, interp.No},
		{"ArrayNameWithoutSubscriptIsTheList", s.ArrayNameWithoutSubscriptIsTheList, interp.No},
		{"SelectPromptNeedsTerminal", s.SelectPromptNeedsTerminal, interp.Yes},
		{"SelectEofIsSuccess", s.SelectEofIsSuccess, interp.No},
		{"SelectTakesUnterminatedReply", s.SelectTakesUnterminatedReply, interp.No},
		{"SelectEofPrintsNewline", s.SelectEofPrintsNewline, interp.No},
		{"SelectEofEndsPromptLine", s.SelectEofEndsPromptLine, interp.No},
		// A menu that is always vertical never consults a width, so the
		// axis does not arise here and is left unanswered.
		{"SelectAssumesUnboundedWidth", s.SelectAssumesUnboundedWidth, interp.Unspecified},
		{"DeclaredNameWithoutValueIsEmpty", s.DeclaredNameWithoutValueIsEmpty, interp.No},
		{"TypesetLocalNeedsKeywordFunction", s.TypesetLocalNeedsKeywordFunction, interp.Yes},
		{"IndirectionYieldsName", s.IndirectionYieldsName, interp.Yes},
		// The brace-range answers: `{01..3}` is `1 2 3`, `{10..1..3}` is
		// `10` and `{1..10..-3}` is `1`, and `{3..1..-1}` keeps the
		// endpoints' order as `3 2 1`.
		{"BraceRangePadsToEndpointWidth", s.BraceRangePadsToEndpointWidth, interp.No},
		{"BraceRangeStepSignHonored", s.BraceRangeStepSignHonored, interp.Yes},
		{"BraceRangeNegativeStepReverses", s.BraceRangeNegativeStepReverses, interp.No},
		{"ArithInvalidOctalDigitIsError", s.ArithInvalidOctalDigitIsError, interp.No},
		{"ArithLeadingZeroIsOctal", s.ArithLeadingZeroIsOctal, interp.Yes},
		{"ArithIntegerOperatorRefusesFloat", s.ArithIntegerOperatorRefusesFloat, interp.Yes},
		{"LastPipelineElementInCurrentShell", s.LastPipelineElementInCurrentShell, interp.Yes},
		// A `jobs` listing: which end it starts from, and whether a job that
		// has already ended appears in it at all. Both split the panel two
		// and two, which is why both are fields.
		// Whether an unassigned subscript is an element.
		{"ArraysAreSparse", s.ArraysAreSparse, interp.Yes},
		{"OperatorDistributesOverStarSubscript", s.OperatorDistributesOverStarSubscript, interp.Yes},
		{"AnnouncesBackgroundJob", s.AnnouncesBackgroundJob, interp.Yes},
		{"ReportsACommandKilledBySignal", s.ReportsACommandKilledBySignal, interp.Yes},
		{"ReportsAnyKilledPipelineElement", s.ReportsAnyKilledPipelineElement, interp.No},
		{"ChildInterruptEndsTheScript", s.ChildInterruptEndsTheScript, interp.Yes},
		{"CdRefusesUnknownOption", s.CdRefusesUnknownOption, interp.Yes},
		{"CdLastPathOptionWins", s.CdLastPathOptionWins, interp.Yes},
		{"BadSetOptionNameFatal", s.BadSetOptionNameFatal, interp.Yes},
		// The two questions come apart here: `set -o nosuchoption` ends the
		// script and `[[ -o nosuchoption ]]` is a quiet false.
		{"UnknownConditionOptionIsAStatus", s.UnknownConditionOptionIsAStatus, interp.No},
		// `set -h` is command tracking — trackall, this shell's name for
		// it — and `set -m` is granted to a script with no terminal.
		{"SetHasTheHLetter", s.SetHasTheHLetter, interp.Yes},
		{"SetHLetterTracksCommands", s.SetHLetterTracksCommands, interp.Yes},
		{"MonitorNeedsATerminal", s.MonitorNeedsATerminal, interp.No},
		{"ReturnOutsideAFunctionIsRefused", s.ReturnOutsideAFunctionIsRefused, interp.No},
		{"LoneDashIsAnOption", s.LoneDashIsAnOption, interp.No},
		{"ReadonlyReassignmentFatalFromCommandString", s.ReadonlyReassignmentFatalFromCommandString, interp.Yes},
		{"ReadonlyReassignmentByDeclarationFatal", s.ReadonlyReassignmentByDeclarationFatal, interp.Yes},
		{"UnsetFunctionChecksTheName", s.UnsetFunctionChecksTheName, interp.Yes},
		{"UnsetFunctionReportsMissing", s.UnsetFunctionReportsMissing, interp.No},
		// What `type` does: whether it follows the sentence with the
		// function itself, and whether `--` ends its options.
		{"TypePrintsFunctionBody", s.TypePrintsFunctionBody, interp.No},
		{"TypeEndsOptionsWithDashDash", s.TypeEndsOptionsWithDashDash, interp.Yes},
		{"TypeNamesTheKindWithDashT", s.TypeNamesTheKindWithDashT, interp.No},
		{"JobsShowBackgroundCommand", s.JobsShowBackgroundCommand, interp.No},
		{"JobsListNewestFirst", s.JobsListNewestFirst, interp.Yes},
		{"JobsListFinishedJobs", s.JobsListFinishedJobs, interp.Yes},
		// ERR and DEBUG but not RETURN. The subshell answers differ on
		// purpose: a command substitution here captures the DEBUG
		// handler's output and not the ERR handler's.
		{"TrapHasErrCondition", s.TrapHasErrCondition, interp.Yes},
		{"TrapHasDebugCondition", s.TrapHasDebugCondition, interp.Yes},
		{"TrapHasReturnCondition", s.TrapHasReturnCondition, interp.No},
		{"ErrTrapRunsInsideFunctions", s.ErrTrapRunsInsideFunctions, interp.Yes},
		{"ErrTrapRunsInSubshells", s.ErrTrapRunsInSubshells, interp.No},
		{"DebugTrapRunsInsideCalls", s.DebugTrapRunsInsideCalls, interp.Yes},
		{"DebugTrapRunsInSubshells", s.DebugTrapRunsInSubshells, interp.Yes},
		// `(trap)` and `$(trap)` keep the parent's listing, EXIT included;
		// a pipeline element and a background job list nothing the parent
		// had.
		{"SubshellKeepsTrapListing", s.SubshellKeepsTrapListing, interp.Yes},
		{"PipelineElementKeepsTrapListing", s.PipelineElementKeepsTrapListing, interp.No},
		{"BackgroundJobKeepsTrapListing", s.BackgroundJobKeepsTrapListing, interp.No},
		{"KeptTrapListingIncludesExit", s.KeptTrapListingIncludesExit, interp.Yes},
		{"SubshellHidesInheritedIgnoredTraps", s.SubshellHidesInheritedIgnoredTraps, interp.No},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.axis, tc.got, tc.want)
		}
	}
}

func TestDiagnostics(t *testing.T) {
	// The four lines `type` prints, each of them this shell's own words.
	if got, want := ksh.Diagnostics().TypeKeyword, "%[1]s is a keyword"; got != want {
		t.Errorf("TypeKeyword = %q, want %q", got, want)
	}
	if got, want := ksh.Diagnostics().TypeExternal, "%[1]s is a tracked alias for %[2]s"; got != want {
		t.Errorf("TypeExternal = %q, want %q", got, want)
	}
	// A name it could not account for is a complaint, so it goes to standard
	// error the way the rest of `whence`'s messages do. Half the panel
	// reports it on standard output instead.
	if ksh.Diagnostics().TypeNotFoundOnStdout {
		t.Error("TypeNotFoundOnStdout = true, want the line on standard error")
	}
	// `type` is `whence -v` here, and a refused option says so — the
	// complaint and the usage line under it both name `whence`.
	if got, want := ksh.Diagnostics().BuiltinComplaintName["type"], "whence"; got != want {
		t.Errorf("BuiltinComplaintName[type] = %q, want %q", got, want)
	}
	if got, want := ksh.Diagnostics().BuiltinUsage["type"], "Usage: whence [-afpqv] name  ..."; got != want {
		t.Errorf("BuiltinUsage[type] = %q, want %q", got, want)
	}
	// ksh93 lists a job that has already ended as "Running" — not a reaping
	// race, it still says so after `wait`. The word ksh uses, rather than
	// the word a shell ought to use.
	if got, want := ksh.Diagnostics().JobDone, " Running"; got != want {
		t.Errorf("JobDone = %q, want %q", got, want)
	}
	// And says something else when it is the one reporting: ksh announces
	// `Done` and then lists the same job as `Running`.
	if got, want := ksh.Diagnostics().JobDoneNotice, " Done"; got != want {
		t.Errorf("JobDoneNotice = %q, want %q", got, want)
	}
	// The separator ksh puts between the job number and the pid is a tab.
	if got, want := ksh.Diagnostics().JobStarted, "[%[1]d]\t%[2]d"; got != want {
		t.Errorf("JobStarted = %q, want %q", got, want)
	}
	if got, want := ksh.Diagnostics().SyntaxStatus(), 3; got != want {
		t.Errorf("syntax-error status = %d, want %d", got, want)
	}
	// A compound command's failed open is reported at the line *before* the
	// redirect's: the same loop bash reports at 5 is 4 here, and a compound
	// written entirely on line 2 is line 1 — which this dialect prints as no
	// line at all.
	if got, want := ksh.Diagnostics().RedirectFailureLine, interp.LineBeforeRedirect; got != want {
		t.Errorf("RedirectFailureLine = %v, want %v", got, want)
	}
}

// TestDerivesFromTheStandardNotFromASibling is the property the package
// comment promises. A preset that inherits from another shell inherits its
// future mistakes; this one starts from POSIX and overrides only what was
// measured.
func TestDerivesFromTheStandardNotFromASibling(t *testing.T) {
	posix := interp.PosixSemantics()
	s := ksh.Semantics()
	if s == posix {
		t.Error("the preset overrides nothing, which cannot be right")
	}
}

// TestApplyRemovesLocal is the one builtin the panel disagrees about. ksh93
// reports it as a command that was not found, which is what removing it from
// the substrate's table produces — asserted through what a script sees rather
// than by inspecting the table, because that is what a script can tell.
func TestApplyRemovesLocal(t *testing.T) {
	const src = `f() { local v=in; }; f`
	run := func(apply bool) (string, int) {
		t.Helper()
		f, err := syntax.Parse(src, ksh.Dialect())
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		sem := ksh.Semantics()
		diag := ksh.Diagnostics()
		r := &interp.Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &diag, Name: "ksh", Dialect: presetDialect()}
		if apply {
			ksh.Apply(r)
		}
		st, err := r.Run(context.Background(), f)
		if err != nil {
			t.Fatal(err)
		}
		return buf.String(), st
	}
	// Without the dialect's adjustment the substrate provides local.
	if out, st := run(false); st != 0 || out != "" {
		t.Fatalf("the substrate should provide local: %q status %d", out, st)
	}
	out, st := run(true)
	if !strings.Contains(out, "local: not found") {
		t.Errorf("ksh93 has no local: got %q", out)
	}
	if st != 127 {
		t.Errorf("status = %d, want 127", st)
	}
}

// TestNoAmpersandRedirect pins the build this panel measures. `&>` is the
// divergence syntax.Dialect names as the reason its fields are called after
// constructs rather than after shells: ksh93u+ 2012 has no `&>` and
// ksh93u+m does, twelve years apart under the same name.
func TestNoAmpersandRedirect(t *testing.T) {
	if ksh.Dialect().AmpersandRedirect {
		t.Error("ksh93u+ 2012 has no &>, and treating it as one operator silently redirects what should have been backgrounded")
	}
}

// TestUnterminatedNamesTheInnermostUnclosedKeyword is ksh93's view, and the
// one with an irregularity in it: a `do` inside a while is named and a `do`
// inside a `for` is not, which is measured rather than derived.
func TestUnterminatedNamesTheInnermostUnclosedKeyword(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"if", "syntax error at line 1: `if' unmatched"},
		{"if true; then echo x", "syntax error at line 1: `then' unmatched"},
		{"if true; then echo x; else echo y", "syntax error at line 1: `else' unmatched"},
		{"while true; do echo x", "syntax error at line 1: `do' unmatched"},
		{"for i in a; do echo x", "syntax error at line 1: `for' unmatched"},
		// The line ksh93 embeds is where the input ran out, not where the
		// construct began — the two differ only across lines, which is why a
		// single-line case could not tell them apart.
		{"if true; then\necho x", "syntax error at line 2: `then' unmatched"},
	} {
		_, err := syntax.Parse(tc.src, ksh.Dialect())
		if got := ksh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestASourcedFileIsNamedByTheBuiltin is ksh93 alone: a diagnostic about a
// file `.` read names `.` rather than the file, where the other three name
// the path.
func TestASourcedFileIsNamedByTheBuiltin(t *testing.T) {
	d := ksh.Diagnostics()
	if !d.SourceFileIsTheBuiltin {
		t.Error("a sourced file should be named by the builtin that read it")
	}
	if got, want := d.SourceFileNaming, interp.SourceBeforeLocation; got != want {
		t.Errorf("got %v, want it named before the location", got)
	}
}

// TestArithmeticFailuresAreTerse is ksh93's shape.
func TestArithmeticFailuresAreTerse(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"echo $((1 2))", "1 2: arithmetic syntax error"},
		{"echo $((1+))", "1+: more tokens expected"},
	} {
		_, err := syntax.Parse(tc.src, ksh.Dialect())
		if got := ksh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestUnexpectedTokensCarryTheLine is ksh93's shape: the line rides inside the
// message, because ksh93 prints no location of its own for a parse failure.
func TestUnexpectedTokensCarryTheLine(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"echo )", "syntax error at line 1: `)' unexpected"},
		{"echo x\necho )", "syntax error at line 2: `)' unexpected"},
	} {
		_, err := syntax.Parse(tc.src, ksh.Dialect())
		if got := ksh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestKshHasItsOwnNamesForThings covers four answers that are ksh93's alone,
// three of them wordings and one a grammar flag.
func TestKshHasItsOwnNamesForThings(t *testing.T) {
	// `times` is a reserved word, so a word after it does not parse at all.
	if _, err := syntax.Parse("times foo", ksh.Dialect()); err == nil {
		t.Error("`times foo` should not parse where times is reserved")
	} else if got, want := ksh.Diagnostics().ParseFailure(err),
		"syntax error at line 1: `foo' unexpected"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// And `times` on its own still parses, which is the half a flag like this
	// is easiest to break.
	if _, err := syntax.Parse("times", ksh.Dialect()); err != nil {
		t.Errorf("`times` alone should still parse: %v", err)
	}

	// An operator ksh93 cannot read is a syntax error naming the character,
	// not a bad substitution.
	_, err := syntax.Parse("echo ${x^^}", ksh.Dialect())
	if got, want := ksh.Diagnostics().ParseFailure(err),
		"syntax error at line 1: `^' unexpected"; got != want {
		t.Errorf("${x^^}: got %q, want %q", got, want)
	}

	d := ksh.Diagnostics()
	if !d.DotNoOperandUnprefixed {
		t.Error("`.` with no operand should print its usage bare")
	}
	if got, want := d.InvalidNumber, "%[1]s: parameter not set"; got != want {
		t.Errorf("InvalidNumber = %q, want %q — a value that is not a number is a name here", got, want)
	}
}

// TestPrintfAnswers covers the two that are ksh93's alone: `\cX` is a control
// character rather than two characters or a full stop, and its output reaches
// the reader before its complaint about the rest.
func TestPrintfAnswers(t *testing.T) {
	s := ksh.Semantics()
	if got, want := s.PrintfBackslashC, interp.PrintfBackslashCControl; got != want {
		t.Errorf("PrintfBackslashC = %v, want %v", got, want)
	}
	if got, want := s.PrintfOutputPrecedesComplaint, interp.Yes; got != want {
		t.Errorf("PrintfOutputPrecedesComplaint = %v, want %v", got, want)
	}
	if got, want := s.PrintfQuote, interp.PrintfQuoteSingle; got != want {
		t.Errorf("PrintfQuote = %v, want %v", got, want)
	}
}

// TestCdAnswers: ksh93 brackets the reason and has one message for both of
// the variables bash names separately.
func TestCdAnswers(t *testing.T) {
	d := ksh.Diagnostics()
	if got, want := d.CdCannotChange, "cd: %[1]s: [%[2]s]"; got != want {
		t.Errorf("CdCannotChange = %q, want %q", got, want)
	}
	if d.CdHomeNotSet != d.CdOldpwdNotSet || d.CdHomeNotSet != "cd: bad directory" {
		t.Errorf("one message for both, got %q and %q", d.CdHomeNotSet, d.CdOldpwdNotSet)
	}
}

// TestKshHasOnlyTheOlderDeclarationName: ksh93 has `typeset` and not
// `declare`, which is the mirror of it having `source` and not `local`.
func TestKshHasOnlyTheOlderDeclarationName(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"typeset", "1"},
		{"declare", "not found"},
	} {
		f, err := syntax.Parse(tc.name+` x=1; echo "$x"`, ksh.Dialect())
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		s, d := ksh.Semantics(), ksh.Diagnostics()
		r := &interp.Runner{Stdout: &out, Stderr: &out, Semantics: &s, Diagnostics: &d, Dialect: presetDialect()}
		ksh.Apply(r)
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), tc.want) {
			t.Errorf("%s: got %q, want it to contain %q", tc.name, out.String(), tc.want)
		}
	}
}

// TestBraceRangeAnswers: the range corners that are ksh93's alone — an
// endpoint's zeros are stripped rather than padding the range, and a written
// step's sign is taken at its word, so a sign pointing away from the far
// endpoint leaves the range one element long.
func TestBraceRangeAnswers(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`echo {01..3}`, "1 2 3"},
		{`echo {-03..3..3}`, "-3 0 3"},
		{`echo {10..1..3}`, "10"},
		{`echo {1..10..-3}`, "1"},
		{`echo {a..e..-1}`, "a"},
		{`echo {3..1..-1}`, "3 2 1"},
	} {
		f, err := syntax.Parse(tc.src, ksh.Dialect())
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		s, d := ksh.Semantics(), ksh.Diagnostics()
		r := &interp.Runner{Stdout: &out, Stderr: &out, Semantics: &s, Diagnostics: &d, Dialect: presetDialect()}
		ksh.Apply(r)
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimSpace(out.String()); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestSelectMenuLayout: ksh93's menu is always one item per line here. It does
// columnize, but on the terminal's *height* rather than its width — measured
// and not built, because a menu long enough to reach it cannot be graded by a
// corpus whose cases do not control LINES.
func TestSelectMenuLayout(t *testing.T) {
	if got, want := ksh.Semantics().SelectLayout, interp.SelectMenuVertical; got != want {
		t.Errorf("SelectLayout = %v, want %v", got, want)
	}
	// The same prompt as bash, and printed only when the input is a terminal
	// — which is why a ksh93 script's transcript has the menu and no prompt.
	if got, want := ksh.Diagnostics().SelectPrompt, "#? "; got != want {
		t.Errorf("SelectPrompt = %q, want %q", got, want)
	}
}

// TestFloatFormatting: ksh93 shows fifteen significant digits and writes a
// whole float as an integer, and it refuses a float where only an integer will
// do rather than truncating.
func TestFloatFormatting(t *testing.T) {
	if !ksh.Dialect().ArithFloat {
		t.Error("ksh93 has floating point")
	}
	d := ksh.Diagnostics()
	if got, want := d.ArithFloatDigits, 15; got != want {
		t.Errorf("ArithFloatDigits = %d, want %d", got, want)
	}
	if d.ArithFloatKeepsPoint {
		t.Error("ksh93 writes a whole float as an integer")
	}
	if got, want := d.ArithInfinity, "inf"; got != want {
		t.Errorf("ArithInfinity = %q, want %q", got, want)
	}
	// A malformed expression is a failed command here rather than a failed
	// parse, the same as in bash.
	if got, want := d.ArithFailureStatus, 1; got != want {
		t.Errorf("ArithFailureStatus = %d, want %d", got, want)
	}
}

// TestPatternGroups: which groups this shell reads, and where.
func TestPatternGroups(t *testing.T) {
	if !ksh.Dialect().ExtendedPattern {
		t.Error("ksh93 has extended patterns wherever a pattern may stand")
	}
	if ksh.Dialect().PatternAlternation {
		t.Error("ksh93 needs a quantifier in front of a group")
	}
}

// TestAParseFailureNamesItsOwnLine: ksh93's parse wording carries the line
// already — `syntax error at line 3` — so the location must not carry it too.
// A *runtime* diagnostic in a script is still prefixed, which is why this is
// not simply the script location being absent.
func TestAParseFailureNamesItsOwnLine(t *testing.T) {
	d := ksh.Diagnostics()
	if !d.ParseFailureNamesItsOwnLine {
		t.Error("ksh93's parse failure names its own line")
	}
	if got, want := d.ForScript().Location, interp.LocationLineWord; got != want {
		t.Errorf("a script's runtime location = %v, want %v — still prefixed", got, want)
	}
}

// TestAParseFailureIsNotPrefixedWithItsOwnLine renders one, because asserting
// the flag says nothing about whether anything reads it.
func TestAParseFailureIsNotPrefixedWithItsOwnLine(t *testing.T) {
	_, err := syntax.Parse("echo one\n{ fi; }\n", ksh.Dialect())
	if err == nil {
		t.Fatal("want a parse failure")
	}
	got := ksh.Diagnostics().ForScript().ParseDiagnostic("s.sh", "", err, "echo one\n{ fi; }\n")
	if strings.Contains(got, "line 2: syntax error") {
		t.Errorf("got %q, want the line named once", got)
	}
	if !strings.Contains(got, "at line 2") {
		t.Errorf("got %q, want the wording to still name the line", got)
	}
	// A runtime diagnostic in a script keeps its prefix.
	if runtime := ksh.Diagnostics().ForScript().Report("s.sh", 2, "nosuchcmd: not found\n"); !strings.Contains(runtime, "line 2:") {
		t.Errorf("got %q, want a runtime diagnostic still prefixed", runtime)
	}
}

// This shell names the process and the signal but never says the command
// back, and it carries its own words for the signal rather than the
// machine's.
func TestAKilledCommandIsSaidInThisShellsOwnWords(t *testing.T) {
	dg := ksh.Diagnostics()
	if got, want := dg.KilledCommandNotice, "%[1]d: %[2]s"; got != want {
		t.Errorf("KilledCommandNotice = %q, want %q", got, want)
	}
	// The three the machine and this shell disagree about most plainly.
	for sig, want := range map[syscall.Signal]string{
		syscall.SIGSEGV: "Memory fault",
		syscall.SIGABRT: "Abort",
		syscall.SIGALRM: "Alarm call",
	} {
		if got := dg.SignalDescriptions[sig]; got != want {
			t.Errorf("words for %v = %q, want %q", sig, got, want)
		}
	}
	// And one they agree the words for, which is still listed — because the
	// machine writes a number after them here and this shell never does.
	if got, want := dg.SignalDescriptions[syscall.SIGKILL], "Killed"; got != want {
		t.Errorf("words for KILL = %q, want %q", got, want)
	}
}

// A substitution's body is numbered from the file, whichever way it is
// written.
func TestASubstitutionsBodyIsNumberedFromTheFile(t *testing.T) {
	if ksh.Diagnostics().BackquotedSubstitutionRestartsLines {
		t.Error("backquotes are numbered from the file here, like $( )")
	}
}

// The letters `$-` starts with, measured from a script file; the letters that
// describe the route come from Runner.Route and are asserted below.
func TestDollarDashStartupLetters(t *testing.T) {
	if got, want := ksh.Semantics().DefaultOptionLetters, "hB"; got != want {
		t.Errorf("DefaultOptionLetters = %q, want %q", got, want)
	}
}

// The panel's holdout on the second route letter. Measured 2026-09-05 on
// 93u+ 2012-08-01: `ksh -c 'echo $-'` reports `chsB` — both letters, where
// bash shows `c` alone and dash and zsh show neither.
//
// Read down its rows and ksh93's rule for `s` is "no script file was named"
// where the other three's is "the program came from standard input". The two
// agree on every other route; this is the one invocation that tells them
// apart, and it is why `s` could not be modeled from the route alone.
func TestDollarDashRouteLetters(t *testing.T) {
	s := ksh.Semantics()
	if got := s.CommandStringShowsCInDollarDash; got != interp.Yes {
		t.Errorf("CommandStringShowsCInDollarDash = %v, want Yes", got)
	}
	if got := s.CommandStringShowsSInDollarDash; got != interp.Yes {
		t.Errorf("CommandStringShowsSInDollarDash = %v, want Yes", got)
	}
	if got := (interp.Semantics{}).CommandStringShowsSInDollarDash; got == interp.Yes {
		t.Error("the substrate says yes, want this to be ksh93's answer and nobody else's")
	}
}

// TestDollarSingleAnswers covers the three `$'…'` axes.
//
// ksh93 decodes `\c` like bash and by different arithmetic — bit 6 toggled
// rather than the low five bits kept — which is invisible over letters and
// decides `$'\c1'`, `q` here and 0x11 there. It also drops the backslash from
// an escape it does not know, where bash keeps both characters.
func TestDollarSingleAnswers(t *testing.T) {
	s := ksh.Semantics()
	if got, want := s.DollarSingleBackslashC, interp.DollarSingleControlToggled; got != want {
		t.Errorf("DollarSingleBackslashC = %v, want %v", got, want)
	}
	if got, want := s.DollarSingleUnknownEscape, interp.DollarSingleUnknownDropsBackslash; got != want {
		t.Errorf("DollarSingleUnknownEscape = %v, want %v", got, want)
	}
	if got, want := s.DollarSingleNulTruncates, interp.Yes; got != want {
		t.Errorf("DollarSingleNulTruncates = %v, want %v", got, want)
	}
}

// A login shell reads ~/.profile whether or not it is going to prompt.
// Measured 2026-09-05 with a scratch HOME, on the script-operand, `-c`,
// standard-input and `-s` routes alike; bash is the panel's holdout and this
// preset takes the majority's answer, which is also the POSIX preset's (#482).
func TestKshReadsTheProfileWithAScriptToRun(t *testing.T) {
	if !ksh.Semantics().LoginProfileWhenNonInteractive {
		t.Error("LoginProfileWhenNonInteractive = false, want true")
	}
	if (interp.Semantics{}).LoginProfileWhenNonInteractive {
		t.Error("the substrate's own answer reads a file out of a home directory, want it not to")
	}
}

// TestARedirectionInAnUncompoundedBodyIsRefused — ksh93's own answer to the
// function-body question, which is neither of the other two: it takes the
// simple command and refuses the redirection, naming the operator.
func TestARedirectionInAnUncompoundedBodyIsRefused(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"f() >out; f", "syntax error at line 1: `>' unexpected"},
		{"f() echo hi >out; f", "syntax error at line 1: `>' unexpected"},
		{"f() 2>&1; f", "syntax error at line 1: `>&' unexpected"},
	} {
		_, err := syntax.Parse(tc.src, ksh.Dialect())
		if err == nil {
			t.Errorf("%q: parsed, want a syntax error", tc.src)
			continue
		}
		if got := ksh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}
	// The command without the redirection, and the redirection on a braced
	// body: both accepted, which is what says this is about the body.
	for _, src := range []string{"f() echo hi; f", "f() { echo hi; } >out; f"} {
		if _, err := syntax.Parse(src, ksh.Dialect()); err != nil {
			t.Errorf("%q: refused: %v", src, err)
		}
	}
}

// TestDollarDashInteractiveStartupLetters, and this is the shell that decides
// the shape of the axis. Measured 2026-09-05: `ksh -i script.sh` reports
// `imBE` where a script reports `hB`, so the `h` is *dropped* — `set -o` says
// `trackall on` for the script and off when interactive — and `rc` comes on,
// which is the `E`. A field of letters to append could not have said any of
// that.
//
// The `m` is deliberately absent from the field. It is a monitor that is
// really running — `set -o` reports `monitor on` under `-i script.sh` in this
// shell and off in bash and zsh — so the letter belongs to the runner's state.
// That the shell turns job control on there and this front end does not is
// recorded in docs/spec/invocation.md as measured and not modeled.
func TestDollarDashInteractiveStartupLetters(t *testing.T) {
	got := ksh.Semantics().InteractiveOptionLetters
	if want := "BE"; got != want {
		t.Errorf("InteractiveOptionLetters = %q, want %q", got, want)
	}
	if strings.ContainsRune(got, 'm') {
		t.Error("the monitor letter must come from the monitor, not from a startup string")
	}
}

// Nothing is said when an interactive shell cannot have job control, which is
// this dialect's answer and not an omission.
//
// ksh93 needs no terminal for the monitor — InteractiveMonitorNeedsATerminal
// is No here — so there is never a moment where it wanted one and could not
// have it, and measured with every stream redirected it writes nothing at all.
func TestNothingIsSaidAboutJobControlAtStartup(t *testing.T) {
	if got := ksh.Diagnostics().NoJobControlAtStartup; got != "" {
		t.Errorf("NoJobControlAtStartup = %q, want empty — this shell says nothing", got)
	}
}

// The job-spec complaint names no spec, which is the shell and not a
// truncation: measured, `jobs %9` and `jobs %nope` produce the same line.
//
// The status is the shared 1, so there is no field for it.
func TestTheJobSpecThatNamesNothing(t *testing.T) {
	if got, want := ksh.Diagnostics().NoSuchJob, "%[1]s: no such job"; got != want {
		t.Errorf("NoSuchJob = %q, want %q", got, want)
	}
	if got := ksh.Diagnostics().NoSuchJobStatus; got != 0 {
		t.Errorf("NoSuchJobStatus = %d, want 0 — the shared 1", got)
	}
}

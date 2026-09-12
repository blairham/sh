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

// The measured answers for zsh live here, in zsh's own package, so that adding
// or correcting a shell never touches the substrate.

func parses(t *testing.T, src string) bool {
	t.Helper()
	_, err := syntax.Parse(src, zsh.Dialect())
	return err == nil
}

func TestGrammar(t *testing.T) {
	// The route decides: measured 2026-09-05, `zsh -c` leaves an alias
	// alone and a script file and standard input both expand it. The prompt
	// is a fourth question and the front end answers it.
	if got, want := zsh.Dialect().ExpandAliases, syntax.RouteFromScriptFile|syntax.RouteOnStandardInput; got != want {
		t.Errorf("ExpandAliases = %v, want %v", got, want)
	}
	if !zsh.Dialect().ExpandAliases.Has(syntax.RouteFromScriptFile) ||
		zsh.Dialect().ExpandAliases.Has(syntax.RouteFromCommandString) {
		t.Error("the two routes that differ are what the set is for")
	}
	// The length may carry an operator here, and this shell is the only one
	// in the panel that reads the pairing as a construct at all: `${#v#a}` is
	// the length of what the trim leaves, where bash, bash 3.2, bash as `sh`,
	// dash and ksh93 all answer `bad substitution`. Pinned as a value rather
	// than only as a behavior, because nothing else names which answer this
	// preset gives — mutation says so.
	if !zsh.Dialect().ParamLengthTakesAnOperator {
		t.Error("ParamLengthTakesAnOperator = false, want true")
	}
	// And the other reading of a colon before an operator is not this
	// shell's: `${v:#p}` is an element exclusion here, not `${v#p}`.
	if zsh.Dialect().ParamColonBeforeTrimIsIgnored {
		t.Error("ParamColonBeforeTrimIsIgnored = true, want false")
	}
	// And a body's newlines are lines of the program: $LINENO after a
	// two-line body reads 6 against a physical 5.
	if !zsh.Dialect().AliasBodyCountsLines {
		t.Error("AliasBodyCountsLines = false, want true")
	}
	// The try-always block is this shell's alone, and the value is pinned
	// rather than only the behavior: nothing else names which answer this
	// preset gives, and nine files in a real plugin tree turn on it (#1216).
	if !zsh.Dialect().TryAlways {
		t.Error("TryAlways = false, want true")
	}
	for _, tc := range []struct {
		src  string
		want bool
	}{
		{`case a in a) echo x;;& esac`, false},
		// An operator zsh does not have parses and fails at *expansion*
		// time: `if false; then echo ${x^^}; fi; echo ok` prints ok in zsh,
		// so the refusal cannot live in the grammar.
		{`echo ${x^^}`, true},
		{`echo ${!x}`, false},
		// The array form is the one scripts reach for — iterating an array by
		// index — and it takes a name and a subscript where the scalar takes
		// only a name. zsh refuses both, and refuses them at *parse* time, so
		// nothing after the line runs. Its own spelling is `${(k)a}`.
		{`echo ${!a[@]}`, false},
		{`echo ${!a[*]}`, false},
		{`function f() { echo x; }`, true},
		{`a=(x y)`, true},
		// Short loops. Measured 2026-09-05 against zsh 5.9.2; the other four
		// panel shells refuse every accepted row here.
		{`while (( i < 2 )) echo $i`, true},
		{`until (( i > 2 )) echo $i`, true},
		{`while (( i < 2 )) { echo $i }`, true},
		{`for i (a b) { echo $i }`, true},
		{`for i (a b) echo $i`, true},
		{`for i (a b); echo $i`, true},
		{`select x (a b) { echo $x }`, true},
		{`for i in a b; echo $i`, true},
		{`for ((i=0;i<2;i++)) echo $i`, true},
		// The body may be left out, which is what makes `while cond; { … }`
		// mean what it means here.
		{`while false`, true},
		{`for i (a b)`, true},
		{`for i`, true},
		// And the header still has to end itself: a word cannot, so `{` is
		// another word of `true` and the `}` closes nothing.
		{`while true { echo hi }`, false},
		{`if true; { echo yes; }`, false},
		{`for i in a b { echo $i }`, false},
		// A word list that neither ends nor is followed by a separator.
		{`for i in a b`, false},
	} {
		if got := parses(t, tc.src); got != tc.want {
			t.Errorf("%q: parses = %v, want %v", tc.src, got, tc.want)
		}
	}
}

func TestSemantics(t *testing.T) {
	// zsh alone: the trap's own last command decides, so
	// `trap "false; exit" 0` exits 1.
	if got, want := zsh.Semantics().ExitInTrapReportsEarlierStatus, interp.No; got != want {
		t.Errorf("ExitInTrapReportsEarlierStatus = %v, want %v", got, want)
	}
	s := zsh.Semantics()
	for _, tc := range []struct {
		axis string
		got  interp.Answer
		want interp.Answer
	}{
		{"SetFTurnsOffGlobbing", s.SetFTurnsOffGlobbing, interp.No},
		// The reading side of the split above: `$-` reports noglob as the
		// capital, because `-F` is zsh's own short spelling of it.
		{"NoglobLetterIsF", s.NoglobLetterIsF, interp.No},
		{"ArithIntegerOperatorRefusesFloat", s.ArithIntegerOperatorRefusesFloat, interp.No},
		// The strict end of both file-comparison questions: -nt and -ot
		// want both files to exist, and a non-numeric -t operand is a
		// plain false.
		{"MissingFileIsOlder", s.MissingFileIsOlder, interp.No},
		{"TerminalTestRequiresANumber", s.TerminalTestRequiresANumber, interp.No},
		// And a lone `-t` is `-t 1` rather than a non-empty string, the one
		// reading this shell shares with ksh93.
		{"BareTerminalTestIsDescriptorOne", s.BareTerminalTestIsDescriptorOne, interp.Yes},
		{"AssignmentUpdatesPipelineStatus", s.AssignmentUpdatesPipelineStatus, interp.No},
		{"TestAndArithmeticUpdatePipelineStatus", s.TestAndArithmeticUpdatePipelineStatus, interp.No},
		{"UnsetEndsTheProducedPipelineStatus", s.UnsetEndsTheProducedPipelineStatus, interp.Yes},
		{"ArrayScalarIsTheWholeArray", s.ArrayScalarIsTheWholeArray, interp.Yes},
		{"ArrayNameWithoutSubscriptIsTheList", s.ArrayNameWithoutSubscriptIsTheList, interp.Yes},
		{"SelectPromptNeedsTerminal", s.SelectPromptNeedsTerminal, interp.No},
		{"SelectEofIsSuccess", s.SelectEofIsSuccess, interp.Yes},
		{"SelectTakesUnterminatedReply", s.SelectTakesUnterminatedReply, interp.Yes},
		{"SelectEofPrintsNewline", s.SelectEofPrintsNewline, interp.No},
		{"SelectEofEndsPromptLine", s.SelectEofEndsPromptLine, interp.Yes},
		{"SelectAssumesUnboundedWidth", s.SelectAssumesUnboundedWidth, interp.Yes},
		{"DeclaredNameWithoutValueIsEmpty", s.DeclaredNameWithoutValueIsEmpty, interp.Yes},
		// The four declaration divergences this shell is alone in: the export
		// letter carrying `-g` under every word but `local`, a valueless
		// declaration writing a standing name back, a plain word over a name
		// really holding an array being fatal, and `readonly -a` declaring the
		// array as well as freezing the name.
		{"ExportLetterDeclaresAGlobal", s.ExportLetterDeclaresAGlobal, interp.Yes},
		{"ValuelessDeclarationOfAHeldNameListsIt", s.ValuelessDeclarationOfAHeldNameListsIt, interp.Yes},
		{"ScalarOverACompoundIsAnInconsistentType", s.ScalarOverACompoundIsAnInconsistentType, interp.Yes},
		{"ReadonlyRecordsTheCompoundAttribute", s.ReadonlyRecordsTheCompoundAttribute, interp.Yes},
		// The same answer as the axis above, and by coincidence rather than
		// by implication: this shell got the re-read for years out of the
		// other question being yes, and ksh93 answers the two differently.
		{"AttributeRereadsTheValueItFinds", s.AttributeRereadsTheValueItFinds, interp.Yes},
		{"TypesetLocalNeedsKeywordFunction", s.TypesetLocalNeedsKeywordFunction, interp.No},
		{"SplitParamExpansion", s.SplitParamExpansion, interp.No},
		{"UnquotedListJoinsOnIFS", s.UnquotedListJoinsOnIFS, interp.No},
		// And the join this shell does perform: an unquoted `@` list
		// reaching a context that keeps no fields joins on the first
		// character of IFS, so `IFS=-; a=(x y z); v=${a[@]}` is `x-y-z`
		// where bash and ksh93 give `x y z`.
		{"UnsplitAtListJoinsOnIFS", s.UnsplitAtListJoinsOnIFS, interp.Yes},
		{"TrailingSeparatorEndsAField", s.TrailingSeparatorEndsAField, interp.Yes},
		{"ArrayBaseIsZero", s.ArrayBaseIsZero, interp.No},
		{"ArrayLiteralSubscriptIsAKey", s.ArrayLiteralSubscriptIsAKey, interp.No},
		{"SubscriptCommaIsARange", s.SubscriptCommaIsARange, interp.Yes},
		{"SubscriptIsAQuotingContext", s.SubscriptIsAQuotingContext, interp.No},
		{"ScalarSubscriptIsACharacter", s.ScalarSubscriptIsACharacter, interp.Yes},
		// A whole subscript on a scalar reaches the value both ways here.
		// That is this shell alone on the length and this shell with both
		// bashes on the slice, which is why the two are separate fields.
		{"WholeSubscriptOnAScalarMeasuresIt", s.WholeSubscriptOnAScalarMeasuresIt, interp.Yes},
		{"WholeSubscriptOnAScalarSlicesIt", s.WholeSubscriptOnAScalarSlicesIt, interp.Yes},
		{"GlobNoMatchIsError", s.GlobNoMatchIsError, interp.Yes},
		// Reached only through `${~spec}` here, and answered rather than left
		// open: that flag globs one expansion and the backslash quotes.
		{"ValueBackslashQuotesWhatFollows", s.ValueBackslashQuotesWhatFollows, interp.Yes},
		{"PositionalListWithNoneIsSet", s.PositionalListWithNoneIsSet, interp.Yes},
		{"ArithLeadingZeroIsOctal", s.ArithLeadingZeroIsOctal, interp.No},
		// A math error inside `(( ))` leaves 2 here and 1 in the rest of the
		// panel, with the same sentence in front of it either way.
		{"ArithCommandErrorStatusIsTwo", s.ArithCommandErrorStatusIsTwo, interp.Yes},
		{"ReadonlyReassignmentFatal", s.ReadonlyReassignmentFatal, interp.Yes},
		{"DollarZeroNamesTheInnermostCall", s.DollarZeroNamesTheInnermostCall, interp.Yes},
		// A `jobs` listing: which end it starts from, and whether a job that
		// has already ended appears in it at all. Both split the panel two
		// and two, which is why both are fields.
		// Whether an unassigned subscript is an element.
		{"ArraysAreSparse", s.ArraysAreSparse, interp.No},
		{"OperatorDistributesOverStarSubscript", s.OperatorDistributesOverStarSubscript, interp.No},
		{"AnnouncesBackgroundJob", s.AnnouncesBackgroundJob, interp.Yes},
		{"ReportsACommandKilledBySignal", s.ReportsACommandKilledBySignal, interp.No},
		{"ReportsAnyKilledPipelineElement", s.ReportsAnyKilledPipelineElement, interp.No},
		{"ChildInterruptEndsTheScript", s.ChildInterruptEndsTheScript, interp.No},
		{"CdRefusesUnknownOption", s.CdRefusesUnknownOption, interp.No},
		{"CdHasQuietOption", s.CdHasQuietOption, interp.Yes},
		{"CdLastPathOptionWins", s.CdLastPathOptionWins, interp.No},
		{"BadSetOptionNameFatal", s.BadSetOptionNameFatal, interp.Yes},
		// The one shell that says anything about a `[[ -o ]]` name it does
		// not have — and answers 3, which is neither of a condition's two.
		{"UnknownConditionOptionIsAStatus", s.UnknownConditionOptionIsAStatus, interp.Yes},
		// `set -h` is a history option here, not command tracking, and
		// `set -m` wants the terminal this shell ties job control to.
		{"SetHasTheHLetter", s.SetHasTheHLetter, interp.Yes},
		{"SetHLetterTracksCommands", s.SetHLetterTracksCommands, interp.No},
		{"MonitorNeedsATerminal", s.MonitorNeedsATerminal, interp.Yes},
		{"ReturnOutsideAFunctionIsRefused", s.ReturnOutsideAFunctionIsRefused, interp.No},
		// And a `return` at the top of a startup file, which every shell in
		// the panel obeys: this one keeps the argument, where bash discards
		// it. Measured through a pty — an rc of `return 3` and one of
		// `false; return 3` both leave `$?` as 3 at the first prompt (#1422).
		{"StartupFileReturnCarriesItsArgument", s.StartupFileReturnCarriesItsArgument, interp.Yes},
		{"LoneDashIsAnOption", s.LoneDashIsAnOption, interp.Yes},
		// One line for a bad option word here, and the reason is the
		// fatality rather than a rule: the loop never reaches the second
		// word. Pinned so that staying on this answer is a decision (#1170).
		{"SetReportsEveryBadOption", s.SetReportsEveryBadOption, interp.No},
		// `set -A` refusing a name that is not one leaves 0 behind when the
		// program came from an argument and 1 from a script file. Measured
		// on every neighboring refusal too, all of which leave 1 (#1172).
		{"SetArrayBadNameLeavesZeroFromCommandString", s.SetArrayBadNameLeavesZeroFromCommandString, interp.Yes},
		// And here, which is what leaves bash alone on the other answer
		// (#1171).
		{"FailedExpansionAbandonsTheLine", s.FailedExpansionAbandonsTheLine, interp.No},
		{"ReadonlyReassignmentByDeclarationFatal", s.ReadonlyReassignmentByDeclarationFatal, interp.Yes},
		{"UnsetFunctionChecksTheName", s.UnsetFunctionChecksTheName, interp.No},
		{"UnsetFunctionReportsMissing", s.UnsetFunctionReportsMissing, interp.Yes},
		// What `type` does: whether it follows the sentence with the
		// function itself, and whether `--` ends its options.
		{"TypePrintsFunctionBody", s.TypePrintsFunctionBody, interp.No},
		{"TypeEndsOptionsWithDashDash", s.TypeEndsOptionsWithDashDash, interp.Yes},
		{"TypeNamesTheKindWithDashT", s.TypeNamesTheKindWithDashT, interp.No},
		{"JobsShowBackgroundCommand", s.JobsShowBackgroundCommand, interp.Yes},
		{"JobsListNewestFirst", s.JobsListNewestFirst, interp.No},
		{"JobsListFinishedJobs", s.JobsListFinishedJobs, interp.No},
		// ERR and DEBUG but not RETURN, and — alone in the panel — both
		// follow the script into subshells and command substitutions.
		{"TrapHasErrCondition", s.TrapHasErrCondition, interp.Yes},
		{"TrapHasDebugCondition", s.TrapHasDebugCondition, interp.Yes},
		{"TrapHasReturnCondition", s.TrapHasReturnCondition, interp.No},
		{"ErrTrapRunsInsideFunctions", s.ErrTrapRunsInsideFunctions, interp.Yes},
		{"ErrTrapRunsInSubshells", s.ErrTrapRunsInSubshells, interp.Yes},
		{"DebugTrapRunsInsideCalls", s.DebugTrapRunsInsideCalls, interp.Yes},
		{"DebugTrapRunsInSubshells", s.DebugTrapRunsInSubshells, interp.Yes},
		// The subshell listing shows nothing inherited — not even an ignore
		// that is still working — while a pipeline element keeps the signal
		// listing and still drops the EXIT trap from it.
		{"SubshellKeepsTrapListing", s.SubshellKeepsTrapListing, interp.No},
		{"PipelineElementKeepsTrapListing", s.PipelineElementKeepsTrapListing, interp.Yes},
		{"BackgroundJobKeepsTrapListing", s.BackgroundJobKeepsTrapListing, interp.No},
		{"KeptTrapListingIncludesExit", s.KeptTrapListingIncludesExit, interp.No},
		{"SubshellHidesInheritedIgnoredTraps", s.SubshellHidesInheritedIgnoredTraps, interp.Yes},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.axis, tc.got, tc.want)
		}
	}
}

func TestDiagnostics(t *testing.T) {
	// The four lines `type` prints, each of them this shell's own words.
	if got, want := zsh.Diagnostics().TypeKeyword, "%[1]s is a reserved word"; got != want {
		t.Errorf("TypeKeyword = %q, want %q", got, want)
	}
	if got, want := zsh.Diagnostics().TypeFunction, "%[1]s is a shell function from zsh"; got != want {
		t.Errorf("TypeFunction = %q, want %q", got, want)
	}
	if !zsh.Diagnostics().TypeNotFoundUnprefixed {
		t.Error("TypeNotFoundUnprefixed = false, want the line written bare")
	}
	// And on standard output, the other half of treating it as an answer:
	// `type nope 1>/dev/null` prints nothing here.
	if !zsh.Diagnostics().TypeNotFoundOnStdout {
		t.Error("TypeNotFoundOnStdout = false, want the line on standard output")
	}
	if got, want := zsh.Diagnostics().SyntaxStatus(), 1; got != want {
		t.Errorf("syntax-error status = %d, want %d", got, want)
	}
	// The same answer dash gives: where the command began.
	if got, want := zsh.Diagnostics().RedirectFailureLine, interp.LineOfCommand; got != want {
		t.Errorf("RedirectFailureLine = %v, want %v", got, want)
	}
	// A denied `set -m` echoes the spelling that asked and fails at 1 —
	// fatally, but that is BadSetOptionNameFatal's answer, not this one's.
	if got, want := zsh.Diagnostics().MonitorDenied, "can't change option: %[1]s"; got != want {
		t.Errorf("MonitorDenied = %q, want %q", got, want)
	}
	if got, want := zsh.Diagnostics().MonitorDeniedStatus, 1; got != want {
		t.Errorf("MonitorDeniedStatus = %d, want %d", got, want)
	}
}

// TestDerivesFromTheStandardNotFromASibling is the property the package
// comment promises. A preset that inherits from another shell inherits its
// future mistakes; this one starts from POSIX and overrides only what was
// measured.
// TestUnknownSignalIsNamedWithOnePrefix pins a wording that only reads right
// because of which verb it is given.
//
// zsh names a signal it does not know with exactly one SIG in front of it,
// however many the operand arrived with: `Q` is SIGQ and `SIGNOPE` is
// SIGNOPE. A format that added one to the operand as written produced
// SIGSIGNOPE for the second.
func TestUnknownSignalIsNamedWithOnePrefix(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`kill -Q 1`, "unknown signal: SIGQ"},
		{`kill -SIGNOPE 1`, "unknown signal: SIGNOPE"},
		{`kill -s signope 1`, "unknown signal: SIGNOPE"},
	} {
		f, err := syntax.Parse(tc.src, zsh.Dialect())
		if err != nil {
			t.Fatalf("parse %q: %v", tc.src, err)
		}
		var errs bytes.Buffer
		sem, dg := zsh.Semantics(), zsh.Diagnostics()
		r := &interp.Runner{Stderr: &errs, Semantics: &sem, Diagnostics: &dg, Name: "zsh", Dialect: presetDialect()}
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatalf("run %q: %v", tc.src, err)
		}
		if got := errs.String(); !strings.Contains(got, tc.want) {
			t.Errorf("%s: got %q, want it to contain %q", tc.src, got, tc.want)
		}
	}
}

// TestWhatZshSaysAndWhereItSaysIt covers three wordings that were each wrong
// in a different way, and are only checkable against zsh's own vector.
func TestWhatZshSaysAndWhereItSaysIt(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// Measured against the zsh the panel resolves. 5.9.2 from
			// Homebrew says -L; Apple's /bin/zsh 5.9 says -l, and probing
			// whichever came first on PATH is how the wrong one shipped.
			"the hint names -L", `kill -Q 1`,
			"type kill -L for a list of signals",
		},
		{
			// zsh is the only shell in the panel that says anything about a
			// shift it survives.
			"a survivable shift still complains", `set -- a; shift 5`,
			"shift count must be <= $#",
		},
		{
			// No `exec` segment: a command that could not be found is the
			// shell's failure rather than the builtin's, and zsh reports it
			// exactly as it reports a bare command word.
			"exec does not name itself", `exec nosuchcmd-xyz`,
			"zsh:1: command not found: nosuchcmd-xyz",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, zsh.Dialect())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			var errs bytes.Buffer
			sem, dg := zsh.Semantics(), zsh.Diagnostics()
			r := &interp.Runner{Stderr: &errs, Semantics: &sem, Diagnostics: &dg, Name: "zsh", Dialect: presetDialect()}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatalf("run: %v", err)
			}
			if got := errs.String(); !strings.Contains(got, tc.want) {
				t.Errorf("got %q, want it to contain %q", got, tc.want)
			}
		})
	}
}

func TestDerivesFromTheStandardNotFromASibling(t *testing.T) {
	posix := interp.PosixSemantics()
	s := zsh.Semantics()
	if s == posix {
		t.Error("the preset overrides nothing, which cannot be right")
	}
}

// TestUnterminatedNamesTheLastTokenRead is zsh's view, which mentions neither
// the construct nor what would have closed it.
func TestUnterminatedNamesTheLastTokenRead(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"if", "parse error near `if'"},
		{"if true; then echo x", "parse error near `x'"},
		{"case a in a) echo hi", "parse error near `hi'"},
	} {
		_, err := syntax.Parse(tc.src, zsh.Dialect())
		if got := zsh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestBorrowedTextReplacesTheShellName is zsh's answer to both halves of the
// naming question, and it calls eval's text something of its own.
func TestBorrowedTextReplacesTheShellName(t *testing.T) {
	d := zsh.Diagnostics()
	for _, tc := range []struct {
		what string
		got  interp.SourceNaming
	}{{"eval", d.EvalNaming}, {"a sourced file", d.SourceFileNaming}} {
		if tc.got != interp.SourceReplacesShell {
			t.Errorf("%s: got %v, want it to replace the shell's name", tc.what, tc.got)
		}
	}
	if got, want := d.EvalSourceName, "(eval)"; got != want {
		t.Errorf("eval is called %q, want %q", got, want)
	}
}

// TestArithmeticFailuresQuoteNothing is zsh's shape: it does not quote the
// expression at all, and it puts the token inside the reason.
// arithRun is what this dialect says about an expression it refused, observed
// from a run.
//
// It used to be observed from a parse, because the expression was read as part
// of reading the file. No shell in the panel does that — the complaint comes
// when the command runs (#865) — so this is now the only route to it, and it
// carries the whole line the shell writes rather than the sentence alone.
func arithRun(t *testing.T, src string) string {
	t.Helper()
	out, _ := answersRun(t, src)
	return out
}

func TestArithmeticFailuresQuoteNothing(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"echo $((1 2))", "zsh:1: bad math expression: operator expected at `2'\n"},
		{"echo $((1+))", "zsh:1: bad math expression: operand expected at end of string\n"},
	} {
		if got := arithRun(t, tc.src); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// A `[[ ]]` comparison whose operand is not an expression abandons the input
// here, which `(( ))` does not.
//
// The contrast is the test. Both constructs complain in the same words, so a
// run that asserted only the sentence would pass with either behavior; what
// separates them is whether the line after runs. Measured from a script file
// against zsh 5.9.2, where `echo two` never appears and the shell leaves at 1
// (#1616).
func TestAnUnreadableConditionOperandAbandonsTheInput(t *testing.T) {
	for _, tc := range []struct {
		name  string
		src   string
		after bool
	}{
		{"a condition abandons it", "echo one; [[ 1+ -eq 0 ]]; echo two", false},
		{"an arithmetic command does not", "echo one; (( 1+ )); echo two", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, zsh.Dialect())
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			var out, errs bytes.Buffer
			sem, dg := zsh.Semantics(), zsh.Diagnostics()
			r := &interp.Runner{
				Stdout: &out, Stderr: &errs,
				Semantics: &sem, Diagnostics: &dg,
				Name: "zsh", Dialect: presetDialect(),
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatalf("run %q: %v", tc.src, err)
			}
			if !strings.Contains(out.String(), "one") {
				t.Errorf("got %q, want the line before the failure to have run", out.String())
			}
			if got := strings.Contains(out.String(), "two"); got != tc.after {
				t.Errorf("line after ran = %v, want %v (output %q)", got, tc.after, out.String())
			}
			if !strings.Contains(errs.String(), "bad math expression") {
				t.Errorf("stderr %q, want the arithmetic complaint", errs.String())
			}
		})
	}
}

// TestWhoIsSpeakingInAConditionSplitsByWhenItFailed pins a distinction that
// looks like an inconsistency until the mechanism shows.
//
// zsh names the builtin in the location for `test -Q x` and does not for
// `test a b c` — the same builtin, two lines apart. `[[ a b c ]]` gives the
// identical wording with no name, and `[[ ]]` is not a builtin at all: an
// expression that never *parsed* is the condition parser's complaint, and one
// that failed while being *evaluated* is the builtin's.
func TestWhoIsSpeakingInAConditionSplitsByWhenItFailed(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"test a b c", "zsh:1: condition expected: b"},
		{"test -Q x", "zsh:test:1: unknown condition: -Q"},
		{"test 1 -eq a", "zsh:test:1: integer expression expected: a"},
		{"[ x", "zsh:[:1: ']' expected"},
	} {
		f, err := syntax.Parse(tc.src, zsh.Dialect())
		if err != nil {
			t.Fatalf("parse %q: %v", tc.src, err)
		}
		var errs bytes.Buffer
		sem, dg := zsh.Semantics(), zsh.Diagnostics()
		r := &interp.Runner{Stderr: &errs, Semantics: &sem, Diagnostics: &dg, Name: "zsh", Dialect: presetDialect()}
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatalf("run %q: %v", tc.src, err)
		}
		if got := strings.TrimSpace(errs.String()); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestAPatternThatIsNotOneNamesTheToken is the case-arm half of the
// unexpected-token family: `;;&` is not zsh's, so it stops at the `&`.
func TestAPatternThatIsNotOneNamesTheToken(t *testing.T) {
	_, err := syntax.Parse("case a in a) echo x;;& esac", zsh.Dialect())
	if got, want := zsh.Diagnostics().ParseFailure(err), "parse error near `&'"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestEveryTargetIsWritten is zsh's multios, and the axis nobody else sets.
func TestEveryTargetIsWritten(t *testing.T) {
	if got, want := zsh.Semantics().RedirectsUseEveryTarget, interp.Yes; got != want {
		t.Errorf("got %v, want a command's output in every file it names", got)
	}
}

// TestNoclobberRefusalCoversAFailedOpen. Under `noclobber` this shell says
// `file exists` about a target it could not open for a reason of its own — a
// directory, a socket, a dangling symlink, a `/dev/tty` in a session with no
// controlling terminal — where the other three report the open's own reason.
// Measured 2026-09-10 against zsh 5.9.2; see interp/noclobberopen.go.
func TestNoclobberRefusalCoversAFailedOpen(t *testing.T) {
	if !zsh.Diagnostics().NoclobberRefusalCoversAFailedOpen {
		t.Error("a failed open under noclobber is reported as the open's reason, want the option's refusal")
	}
}

// TestPrintfAnswers: zsh stops the output at `\c`, which is the answer that
// looks like ksh93's until the bytes are read.
func TestPrintfAnswers(t *testing.T) {
	s := zsh.Semantics()
	if got, want := s.ReadTrailingEscapedSeparator,
		interp.ReadTrailingEscapedSeparatorTrimmed; got != want {
		t.Errorf("ReadTrailingEscapedSeparator = %v, want %v", got, want)
	}
	if got, want := s.PrintfBackslashC, interp.PrintfBackslashCStops; got != want {
		t.Errorf("PrintfBackslashC = %v, want %v", got, want)
	}
	if got, want := s.PrintfReportsBadNumber, interp.No; got != want {
		t.Errorf("PrintfReportsBadNumber = %v, want %v", got, want)
	}
}

// TestCdAnswers: zsh moves silently and names the reason before the operand,
// which is the reverse of everyone else.
func TestCdAnswers(t *testing.T) {
	s, d := zsh.Semantics(), zsh.Diagnostics()
	if got, want := s.CdDashPrintsTheDirectory, interp.No; got != want {
		t.Errorf("CdDashPrintsTheDirectory = %v, want %v", got, want)
	}
	if got, want := s.CdWithoutHomeIsAnError, interp.No; got != want {
		t.Errorf("CdWithoutHomeIsAnError = %v, want %v", got, want)
	}
	if got, want := d.CdCannotChange, "%[2]s: %[1]s"; got != want {
		t.Errorf("CdCannotChange = %q, want %q", got, want)
	}
}

// TestGetoptsAnswers: zsh is the one that empties OPTARG rather than unsetting
// it, and the one that carries on inside a word when OPTIND is assigned.
func TestGetoptsAnswers(t *testing.T) {
	s := zsh.Semantics()
	if got, want := s.GetoptsClearsOptarg, interp.Yes; got != want {
		t.Errorf("GetoptsClearsOptarg = %v, want %v", got, want)
	}
	if got, want := s.GetoptsAssignmentRestartsWord, interp.No; got != want {
		t.Errorf("GetoptsAssignmentRestartsWord = %v, want %v", got, want)
	}
}

// TestSelectMenuLayout: zsh packs the menu into columns always, so even three
// items share a line, and it asks with the same two characters as the others
// in the other order.
func TestSelectMenuLayout(t *testing.T) {
	if got, want := zsh.Semantics().SelectLayout, interp.SelectMenuColumns; got != want {
		t.Errorf("SelectLayout = %v, want %v", got, want)
	}
	if got, want := zsh.Diagnostics().SelectPrompt, "?# "; got != want {
		t.Errorf("SelectPrompt = %q, want %q", got, want)
	}
}

// TestPipelineStatusName: zsh's name for the same record is the lowercase one,
// and the uppercase name is nothing here.
func TestPipelineStatusName(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`false | true | false; echo "[${pipestatus[@]}]"`, "[1 0 1]"},
		{`false | true | false; echo "[${PIPESTATUS[@]}]"`, "[]"},
	} {
		f, err := syntax.Parse(tc.src, zsh.Dialect())
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		s, d := zsh.Semantics(), zsh.Diagnostics()
		r := &interp.Runner{Name: preset.Name, Stdout: &out, Stderr: &out, Semantics: &s, Diagnostics: &d, Dialect: presetDialect()}
		zsh.Apply(r)
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(out.String()) != tc.want {
			t.Errorf("%s: got %q, want %q", tc.src, out.String(), tc.want)
		}
	}
}

// TestRegexMatchLeavesTheBashNameAlone: zsh's `=~` keeps its captures under
// names and shapes of its own, and the bash-style array is nothing here —
// measured: after a successful match the uppercase name stays unset.
func TestRegexMatchLeavesTheBashNameAlone(t *testing.T) {
	f, err := syntax.Parse(`[[ abcd =~ (b)(c) ]]; echo "st=$? [${BASH_REMATCH[@]}]"`, zsh.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	s, d := zsh.Semantics(), zsh.Diagnostics()
	r := &interp.Runner{Name: preset.Name, Stdout: &out, Stderr: &out, Semantics: &s, Diagnostics: &d, Dialect: presetDialect()}
	zsh.Apply(r)
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "st=0 []" {
		t.Errorf("got %q, want the match to succeed and the name to stay unset", out.String())
	}
}

// TestCloseBraceIsReservedEverywhere: the grammar rule that makes zsh's brace
// groups look different from the other three. It is documented in
// docs/spec/semantics.md as measured, and was measured and never built — the
// parser rejected `{ echo hi }`, which zsh runs.
func TestCloseBraceIsReservedEverywhere(t *testing.T) {
	if !zsh.Dialect().CloseBraceAlwaysReserved {
		t.Error("zsh should reserve `}` wherever a word may stand")
	}
	f, err := syntax.Parse(`{ echo hi }`, zsh.Dialect())
	if err != nil {
		t.Fatalf("zsh should accept a group with no terminator: %v", err)
	}
	var out bytes.Buffer
	s, d := zsh.Semantics(), zsh.Diagnostics()
	r := &interp.Runner{Name: preset.Name, Stdout: &out, Stderr: &out, Semantics: &s, Diagnostics: &d, Dialect: presetDialect()}
	zsh.Apply(r)
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "hi" {
		t.Errorf("got %q, want hi", out.String())
	}
	// And the other half of the same rule.
	if _, err := syntax.Parse(`echo }`, zsh.Dialect()); err == nil {
		t.Error("`echo }` should be a syntax error where `}` is always reserved")
	}
}

// TestFloatFormatting: zsh shows seventeen significant digits and keeps a
// point on a whole float, so the same arithmetic reads differently from
// ksh93's.
func TestFloatFormatting(t *testing.T) {
	if !zsh.Dialect().ArithFloat {
		t.Error("zsh has floating point")
	}
	d := zsh.Diagnostics()
	if got, want := d.ArithFloatDigits, 17; got != want {
		t.Errorf("ArithFloatDigits = %d, want %d", got, want)
	}
	if !d.ArithFloatKeepsPoint {
		t.Error("zsh keeps a point on a whole float")
	}
	if got, want := d.ArithInfinity, "Inf"; got != want {
		t.Errorf("ArithInfinity = %q, want %q", got, want)
	}
}

// TestPatternGroups: which groups this shell reads, and where.
func TestPatternGroups(t *testing.T) {
	if zsh.Dialect().ExtendedPattern || zsh.Dialect().ExtendedPatternInCondition {
		t.Error("zsh has no extended patterns; a quantifier before a group is an ordinary character")
	}
	if !zsh.Dialect().PatternAlternation {
		t.Error("zsh takes a bare group")
	}
}

// TestACommandStringIsReadWhole: zsh parses all of a `-c` command before
// running any of it, where the other three run each line as they reach it. A
// script is read a line at a time in all four, so this is about the command
// string alone.
func TestACommandStringIsReadWhole(t *testing.T) {
	if !zsh.Diagnostics().CommandStringParsedWhole {
		t.Error("zsh reads a command string whole")
	}
}

// TestAProgramOnStandardInputSurvivesAParseFailure: the other route this
// shell reads differently. A line that will not parse is reported and the
// next line is read anyway, so `printf 'echo one\n{ fi; }\necho three\n' |
// zsh` prints three and exits 0 where the other three stop. Only on that
// route: the same program in a file stops this shell too, which is why the
// answer cannot live with the parse status.
func TestAProgramOnStandardInputSurvivesAParseFailure(t *testing.T) {
	if !zsh.Diagnostics().StdinProgramSurvivesAParseFailure {
		t.Error("zsh reads on past a parse failure on standard input")
	}
}

// TestTheRefusalOfANonBuiltinNamesNoBuiltin: zsh names the speaking builtin in
// a diagnostic's location — `zsh:cd:1:`, `zsh:shift:1:` — and does not here.
// The message is about a name that is *not* a builtin, so there is no builtin
// speaking, and naming `builtin` there would be naming the wrong one.
func TestTheRefusalOfANonBuiltinNamesNoBuiltin(t *testing.T) {
	f, err := syntax.Parse(`builtin ls`, zsh.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	s, d := zsh.Semantics(), zsh.Diagnostics()
	r := &interp.Runner{Name: preset.Name, Stdout: &out, Stderr: &out, Semantics: &s, Diagnostics: &d, Dialect: presetDialect()}
	zsh.Apply(r)
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(out.String())
	if !strings.Contains(got, "no such builtin: ls") {
		t.Errorf("got %q, want zsh's wording", got)
	}
	if strings.Contains(got, ":builtin:") {
		t.Errorf("got %q, want no builtin named in the location", got)
	}
	// The contrast: a builtin that really is speaking is named.
	out.Reset()
	f, err = syntax.Parse(`shift 99`, zsh.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	r = &interp.Runner{Name: preset.Name, Stdout: &out, Stderr: &out, Semantics: &s, Diagnostics: &d, Dialect: presetDialect()}
	zsh.Apply(r)
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, ":shift:") {
		t.Errorf("got %q, want the speaking builtin named", got)
	}
}

// This shell says nothing at all about a signal that ended a command, so it
// has no wording for one.
func TestAKilledCommandIsNotSaidAtAll(t *testing.T) {
	if got := zsh.Diagnostics().KilledCommandNotice; got != "" {
		t.Errorf("KilledCommandNotice = %q, want nothing — this shell stays quiet", got)
	}
}

// A substitution's body is numbered from the file, whichever way it is
// written.
func TestASubstitutionsBodyIsNumberedFromTheFile(t *testing.T) {
	if zsh.Diagnostics().BackquotedSubstitutionRestartsLines {
		t.Error("backquotes are numbered from the file here, like $( )")
	}
}

// This shell names the function a message came from rather than the file,
// and counts the line within it.
func TestDiagnosticsNameTheFunction(t *testing.T) {
	if !zsh.Diagnostics().LocationNamesTheFunction {
		t.Error("a message from inside a function is named for the function here")
	}
}

// The letters `$-` starts with, measured identical under -c, a script file
// and standard input — drawn from zsh's own single-letter option namespace,
// which shares almost nothing with the other shells'.
func TestDollarDashStartupLetters(t *testing.T) {
	if got, want := zsh.Semantics().DefaultOptionLetters, "569X"; got != want {
		t.Errorf("DefaultOptionLetters = %q, want %q", got, want)
	}
}

// Neither route letter under `-c`: measured 2026-09-05 on 5.9.2, `zsh -c
// 'echo $-'` reports `569X` and nothing more, where bash and ksh93 add `c`.
// The `s` of the standard-input route is unanimous and comes from
// Runner.Route, so there is no axis for it here.
func TestDollarDashRouteLetters(t *testing.T) {
	s := zsh.Semantics()
	if got := s.CommandStringShowsCInDollarDash; got != interp.No {
		t.Errorf("CommandStringShowsCInDollarDash = %v, want No", got)
	}
	if got := s.CommandStringShowsSInDollarDash; got != interp.No {
		t.Errorf("CommandStringShowsSInDollarDash = %v, want No", got)
	}
}

// TestDollarSingleAnswers covers the three `$'…'` axes, and zsh is the
// minority on all three.
//
// It is the only shell with `$'…'` and no `\c` in it, so `$'\cA'` is the two
// characters `cA`; and its strings are counted rather than terminated, so a
// decoded NUL is a byte in the middle of a word rather than the end of one.
func TestDollarSingleAnswers(t *testing.T) {
	s := zsh.Semantics()
	if got, want := s.DollarSingleBackslashC, interp.DollarSingleControlAbsent; got != want {
		t.Errorf("DollarSingleBackslashC = %v, want %v", got, want)
	}
	if got, want := s.DollarSingleUnknownEscape, interp.DollarSingleUnknownDropsBackslash; got != want {
		t.Errorf("DollarSingleUnknownEscape = %v, want %v", got, want)
	}
	if got, want := s.DollarSingleNulTruncates, interp.No; got != want {
		t.Errorf("DollarSingleNulTruncates = %v, want %v", got, want)
	}
}

// A login shell reads ~/.profile whether or not it is going to prompt.
// Measured 2026-09-05 with a scratch HOME, on the script-operand, `-c`,
// standard-input and `-s` routes alike; bash is the panel's holdout and this
// preset takes the majority's answer, which is also the POSIX preset's (#482).
func TestZshReadsTheProfileWithAScriptToRun(t *testing.T) {
	if !zsh.Semantics().LoginProfileWhenNonInteractive {
		t.Error("LoginProfileWhenNonInteractive = false, want true")
	}
	if (interp.Semantics{}).LoginProfileWhenNonInteractive {
		t.Error("the substrate's own answer reads a file out of a home directory, want it not to")
	}
}

// The panel's holdout on what a refused `set` option reports: 1 where bash,
// dash and ksh93 report 2. Measured 2026-09-05 on `-q`, `-j`, `-z` and `-A` —
// the letters all six shells refuse — and on `set -o nosuchoption`, at an
// invocation and inside a script alike, which is why one value answers both
// spellings (#483).
func TestZshRefusesASetOptionAtOne(t *testing.T) {
	if got := zsh.Diagnostics().SetInvalidOptionStatus; got != 1 {
		t.Errorf("SetInvalidOptionStatus = %d, want 1", got)
	}
	if got := (interp.Diagnostics{}).SetInvalidOptionStatus; got != 0 {
		t.Errorf("the substrate answers %d, want nothing — zero means the 2 the other three report", got)
	}
}

// TestAFunctionBodyThatNeverBeganIsLocatedByNameAlone — zsh drops the line for
// this one failure and keeps it for every other, including the end-of-input
// failures it looks most like.
//
// It is the whole diagnostic rather than the wording, because the location is
// what moves: ParseFailure says the same sentence either way.
func TestAFunctionBodyThatNeverBeganIsLocatedByNameAlone(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"f() ;", "zsh: parse error near `;'\n"},
		{"f()", "zsh: parse error near `()'\n"},
		{"f() &", "zsh: parse error near `&'\n"},
		// The controls: a body that began and ran out, and an end of input
		// with nothing to do with a function. Both keep the line.
		{"f() {", "zsh:1: parse error near `{'\n"},
		{"if true", "zsh:1: parse error near `true'\n"},
		{"echo hi\nif true", "zsh:2: parse error near `true'\n"},
	} {
		_, err := syntax.Parse(tc.src, zsh.Dialect())
		if err == nil {
			t.Errorf("%q: parsed, want a syntax error", tc.src)
			continue
		}
		if got := zsh.Diagnostics().ParseDiagnostic("zsh", "-c", err, tc.src); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestDollarDashInteractiveStartupLetters. Measured 2026-09-05 on zsh 5.9.2:
// `zsh -i script.sh` reports `569XZi`, so the interactive set is the same four
// plus the line-editor letter. `i` comes from the runner.
func TestDollarDashInteractiveStartupLetters(t *testing.T) {
	if got, want := zsh.Semantics().InteractiveOptionLetters, "569XZ"; got != want {
		t.Errorf("InteractiveOptionLetters = %q, want %q", got, want)
	}
}

// Nothing is said when an interactive shell cannot have job control, which is
// this dialect's answer and not an omission.
//
// zsh drops the monitor without a word. Measured with every stream redirected
// on `-i script.sh`, `-i -c` and `-i -s`: `$-` loses its `m` and nothing is
// written to either stream.
func TestNothingIsSaidAboutJobControlAtStartup(t *testing.T) {
	if got := zsh.Diagnostics().NoJobControlAtStartup; got != "" {
		t.Errorf("NoJobControlAtStartup = %q, want empty — this shell says nothing", got)
	}
}

// The wording for a job spec that names nothing is the shared one; the status
// is not.
//
// Measured: `jobs %9` reports 127 — the status of a command that is not there,
// which is what this shell takes a job that is not there to be, and the same
// number its own `wait` gives for the same question.
func TestTheJobSpecThatNamesNothing(t *testing.T) {
	if got := zsh.Diagnostics().NoSuchJob; got != "" {
		t.Errorf("NoSuchJob = %q, want empty — the shared wording", got)
	}
	if got, want := zsh.Diagnostics().NoSuchJobStatus, 127; got != want {
		t.Errorf("NoSuchJobStatus = %d, want %d", got, want)
	}
}

// TestABareArrayNameIsTheElements — this shell alone reads an array named
// without a subscript as the array itself: `$a` is `${a[@]}` unquoted and the
// joined value in quotes (#929).
//
// Here rather than in the substrate's own suite because two of the rows are
// this grammar's: `${a:#p}` and `${a:|b}` do not parse anywhere else, and they
// are the half of the bug that had already shipped — `:#` landed correct on
// scalars and on `(@)` arrays and silently did nothing on a bare one.
//
// Every want is the exact field count and the exact values. The bug returns a
// plausible string at status 0, so "no error" and "contains foo" both pass
// against it.
func TestABareArrayNameIsTheElements(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The bare name, unquoted: one field per element.
		{`a=(one two); set -- $a; echo "n=$# [$1][$2]"`, "n=2 [one][two]"},
		{`a=(one two); for f in $a; do printf "<%s>" "$f"; done; echo`, "<one><two>"},
		// Not a join followed by a split: with IFS empty a join would give
		// one field `xyz`.
		{`IFS=; a=(x y z); set -- $a; echo "n=$# [$1][$3]"`, "n=3 [x][z]"},
		{`IFS=-; a=(x y z); set -- $a; echo "n=$# [$1][$3]"`, "n=3 [x][z]"},
		// Quoted, one field joined on the first character of IFS — not a
		// hard space (#854) — and the same in a context that never splits.
		{`IFS=-; a=(x y z); set -- "$a"; echo "n=$# [$1]"`, "n=1 [x-y-z]"},
		{`IFS=-; a=(x y z); v=$a; echo "[$v]"`, "[x-y-z]"},
		{`IFS=-; a=(x y z); case $a in "x-y-z") echo joined;; *) echo split;; esac`, "joined"},
		// The operators inherit the subject.
		{`a=(one two); set -- ${a#o}; echo "n=$# [$1][$2]"`, "n=2 [ne][two]"},
		{`a=(one two three); set -- ${a:1}; echo "n=$# [$1][$2]"`, "n=2 [two][three]"},
		// A slice of a one-element array is no element at all, where a
		// substring of the same name would be `bcdef`. The row that says the
		// question is asked at one element too.
		{`a=(abcdef); set -- ${a:1}; echo "n=$#"`, "n=0"},
		// `${a:#p}` filters elements. Joined first it matches nothing and
		// hands the whole array back, which is the silent no-op.
		{`a=(1 2 3); set -- ${a:#2}; echo "n=$# [$1][$2]"`, "n=2 [1][3]"},
		{`a=(foo bar baz); set -- ${a:#ba*}; echo "n=$# [$1]"`, "n=1 [foo]"},
		{`a=(x y z); b=(y); set -- ${a:|b}; echo "n=$# [$1][$2]"`, "n=2 [x][z]"},
		// A `-` test that came to the *parameter* yields the parameter, so
		// it yields the elements too.
		{`a=(one two); set -- ${a:-d}; echo "n=$# [$1][$2]"`, "n=2 [one][two]"},
		{`unset a; set -- ${a:-d}; echo "n=$# [$1]"`, "n=1 [d]"},
		// A flag group answers for itself and must not be rewritten under it.
		{`a=(foo bar baz); set -- ${(@)a:#ba*}; echo "n=$# [$1]"`, "n=1 [foo]"},
		{`a=(one two); set -- "${(j:-:)a}"; echo "n=$# [$1]"`, "n=1 [one-two]"},
		// Quoted, `:#` matches the joined string and the array survives —
		// unchanged by the axis, and the guard that says it stayed that way.
		{`a=(foo bar baz); set -- "${a:#ba*}"; echo "n=$# [$1]"`, "n=1 [foo bar baz]"},
		// A one-element array and an empty one agree under both readings.
		{`a=(only); set -- $a; echo "n=$# [$1]"`, "n=1 [only]"},
		{`a=(); set -- $a; echo "n=$#"`, "n=0"},
		// A scalar is not an array, and this shell does not split one.
		{`v="x y"; set -- $v; echo "n=$# [$1]"`, "n=1 [x y]"},
		{`v=abcdef; set -- ${v:1}; echo "n=$# [$1]"`, "n=1 [bcdef]"},
		// `${#a}` is its own axis and keeps counting elements.
		{`a=(one two); echo "[${#a}]"`, "[2]"},
	} {
		f, err := syntax.Parse(tc.src, zsh.Dialect())
		if err != nil {
			t.Fatalf("%s: %v", tc.src, err)
		}
		var out bytes.Buffer
		s, d := zsh.Semantics(), zsh.Diagnostics()
		r := &interp.Runner{Name: preset.Name, Stdout: &out, Stderr: &out, Semantics: &s, Diagnostics: &d, Dialect: presetDialect()}
		zsh.Apply(r)
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatalf("%s: %v", tc.src, err)
		}
		if got := strings.TrimSpace(out.String()); got != tc.want {
			t.Errorf("%s:\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

// TestAnUnquotedListKeepsItsElementsWhole — the two stages that follow an
// expansion are this shell's plainest disagreement with the others, and the
// whole-array path was performing both without asking: every element went
// through field splitting and none was ever glob-escaped (#981).
//
// Here rather than in the substrate's suite because the answers are this
// dialect's: it splits no unquoted parameter expansion and reads no
// expansion's result as a pattern, so it is the one shell where the bug is
// visible. The rows #929 added avoid separators inside elements, which is why
// they passed under it.
//
// Every want is the exact field count and the exact values, because the bug
// hands back the same characters in a different number of fields.
func TestAnUnquotedListKeepsItsElementsWhole(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		// The element holds a separator, so a shell that split would give
		// three fields where this one gives two.
		{`a=("b 2" c); set -- ${a[@]}; echo "n=$# [$1][$2]"`, "n=2 [b 2][c]"},
		{`set -- "p q" r; set -- $@; echo "n=$# [$1][$2]"`, "n=2 [p q][r]"},
		{`a=("b 2" c); set -- $a; echo "n=$# [$1][$2]"`, "n=2 [b 2][c]"},
		{`a=("b 2" c); for f in ${a[@]}; do printf "<%s>" "$f"; done; echo`, "<b 2><c>"},
		// Against the separators as they stand, not against whitespace.
		{`IFS=-; a=("x-y" z); set -- ${a[@]}; echo "n=$# [$1][$2]"`, "n=2 [x-y][z]"},
		{`IFS=-; set -- "x-y" z; set -- $@; echo "n=$# [$1][$2]"`, "n=2 [x-y][z]"},
		// The operators are on the same path and inherit the answer.
		{`a=("b 2" c); set -- ${a[@]:0:2}; echo "n=$# [$1][$2]"`, "n=2 [b 2][c]"},
		{`a=("b 2" c); set -- ${a[@]#q}; echo "n=$# [$1][$2]"`, "n=2 [b 2][c]"},
		// An empty element is no field, which is not a splitting question:
		// it holds here, where nothing splits, exactly as it does elsewhere.
		{`a=("" x); set -- ${a[@]}; echo "n=$# [$1]"`, "n=1 [x]"},
		{`set -- "" x; set -- $@; echo "n=$# [$1]"`, "n=1 [x]"},
		// The result of an expansion is not a pattern here, so an element
		// that looks like one is handed over as its own text — with a file
		// in the directory that it would otherwise have matched.
		{`: > zz1; a=("zz*" other); set -- ${a[@]}; echo "n=$# [$1][$2]"`, "n=2 [zz*][other]"},
		{`: > zz1; set -- "zz*" other; set -- $@; echo "n=$# [$1][$2]"`, "n=2 [zz*][other]"},
		// Quoted is unchanged, which is what says the fix is about the
		// unquoted stages and not about the elements.
		{`a=("b 2" c); set -- "${a[@]}"; echo "n=$# [$1][$2]"`, "n=2 [b 2][c]"},
	} {
		f, err := syntax.Parse(tc.src, zsh.Dialect())
		if err != nil {
			t.Fatalf("%s: %v", tc.src, err)
		}
		var out bytes.Buffer
		s, d := zsh.Semantics(), zsh.Diagnostics()
		r := &interp.Runner{
			Name:   preset.Name,
			Stdout: &out, Stderr: &out, Semantics: &s, Diagnostics: &d,
			Dialect: presetDialect(), Dir: dir,
		}
		zsh.Apply(r)
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatalf("%s: %v", tc.src, err)
		}
		if got := strings.TrimSpace(out.String()); got != tc.want {
			t.Errorf("%s:\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

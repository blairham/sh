// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The measured answers for bash live here, in bash's own package, so that adding
// or correcting a shell never touches the substrate.

func parses(t *testing.T, src string) bool {
	t.Helper()
	_, err := syntax.Parse(src, bash.Dialect())
	return err == nil
}

func TestGrammar(t *testing.T) {
	// Expands on no route at all without `shopt -s expand_aliases`, which
	// is not modeled: measured on all three. The prompt is a different
	// question and the front end answers it.
	if got, want := bash.Dialect().ExpandAliases, syntax.RouteOnNoRoute; got != want {
		t.Errorf("ExpandAliases = %v, want %v", got, want)
	}
	// And a body's newlines are *not* lines of the program: this shell
	// alone leaves the whole of an expanded body on the alias word's
	// line, so $LINENO after a two-line body reads its physical 5.
	if bash.Dialect().AliasBodyCountsLines {
		t.Error("AliasBodyCountsLines = true, want false")
	}
	for _, tc := range []struct {
		src  string
		want bool
	}{
		{`case a in a) echo x;;& esac`, true},
		{`echo ${x^^}`, true},
		{`echo ${!x}`, true},
		{`function f() { echo x; }`, true},
		{`a=(x y)`, true},
		// The brace body belongs to a `for` or `select` and is core; the short
		// loops around it are one other shell's. Measured 2026-09-05 on
		// bash 3.2.57 and 5.3.15, which refuse all four of these.
		{`for i in a b; { echo $i; }`, true},
		{`for i (a b) { echo $i; }`, false},
		{`while (( i < 2 )) echo $i`, false},
		{`for i in a b; echo $i`, false},
		{`while false`, false},
	} {
		if got := parses(t, tc.src); got != tc.want {
			t.Errorf("%q: parses = %v, want %v", tc.src, got, tc.want)
		}
	}
}

func TestSemantics(t *testing.T) {
	// A bare `exit` in an EXIT trap reports the status the trap was entered
	// with, not the trap's own last command.
	if got, want := bash.Semantics().ExitInTrapReportsEarlierStatus, interp.Yes; got != want {
		t.Errorf("ExitInTrapReportsEarlierStatus = %v, want %v", got, want)
	}
	s := bash.Semantics()
	for _, tc := range []struct {
		axis string
		got  interp.Answer
		want interp.Answer
	}{
		{"SetFTurnsOffGlobbing", s.SetFTurnsOffGlobbing, interp.Yes},
		{"NoglobLetterIsF", s.NoglobLetterIsF, interp.Yes},
		{"AssignmentUpdatesPipelineStatus", s.AssignmentUpdatesPipelineStatus, interp.Yes},
		{"UnsetEndsTheProducedPipelineStatus", s.UnsetEndsTheProducedPipelineStatus, interp.No},
		{"ArrayScalarIsTheWholeArray", s.ArrayScalarIsTheWholeArray, interp.No},
		{"ArrayNameWithoutSubscriptIsTheList", s.ArrayNameWithoutSubscriptIsTheList, interp.No},
		{"SelectPromptNeedsTerminal", s.SelectPromptNeedsTerminal, interp.No},
		{"SelectEofIsSuccess", s.SelectEofIsSuccess, interp.No},
		{"SelectTakesUnterminatedReply", s.SelectTakesUnterminatedReply, interp.No},
		{"SelectEofPrintsNewline", s.SelectEofPrintsNewline, interp.Yes},
		{"SelectEofEndsPromptLine", s.SelectEofEndsPromptLine, interp.No},
		{"SelectAssumesUnboundedWidth", s.SelectAssumesUnboundedWidth, interp.No},
		{"DeclaredNameWithoutValueIsEmpty", s.DeclaredNameWithoutValueIsEmpty, interp.No},
		// An attribute waits for the next assignment here rather than
		// re-reading what the name already holds: `FOO=bar; typeset -i FOO`
		// still reads `bar`, and `d=MiXeD; typeset -u d` still reads MiXeD.
		{"AttributeRereadsTheValueItFinds", s.AttributeRereadsTheValueItFinds, interp.No},
		{"TypesetLocalNeedsKeywordFunction", s.TypesetLocalNeedsKeywordFunction, interp.No},
		{"BraceExpansion", s.BraceExpansion, interp.Yes},
		{"IndirectionYieldsName", s.IndirectionYieldsName, interp.No},
		{"ReadonlyReassignmentFatal", s.ReadonlyReassignmentFatal, interp.No},
		{"RegexQuotingMakesLiteral", s.RegexQuotingMakesLiteral, interp.Yes},
		// The file comparisons: a missing file counts as older, and a -t
		// operand that is not a number draws the integer complaint.
		{"MissingFileIsOlder", s.MissingFileIsOlder, interp.Yes},
		{"TerminalTestRequiresANumber", s.TerminalTestRequiresANumber, interp.Yes},
		{"ArithLeadingZeroIsOctal", s.ArithLeadingZeroIsOctal, interp.Yes},
		// A `jobs` listing: which end it starts from, and whether a job that
		// has already ended appears in it at all. Both split the panel two
		// and two, which is why both are fields.
		// Whether an unassigned subscript is an element.
		{"ArraysAreSparse", s.ArraysAreSparse, interp.Yes},
		{"OperatorDistributesOverStarSubscript", s.OperatorDistributesOverStarSubscript, interp.Yes},
		{"AnnouncesBackgroundJob", s.AnnouncesBackgroundJob, interp.Yes},
		{"ReportsACommandKilledBySignal", s.ReportsACommandKilledBySignal, interp.Yes},
		{"ReportsAnyKilledPipelineElement", s.ReportsAnyKilledPipelineElement, interp.No},
		{"ChildInterruptEndsTheScript", s.ChildInterruptEndsTheScript, interp.No},
		{"CdRefusesUnknownOption", s.CdRefusesUnknownOption, interp.Yes},
		{"CdLastPathOptionWins", s.CdLastPathOptionWins, interp.Yes},
		{"BadSetOptionNameFatal", s.BadSetOptionNameFatal, interp.No},
		// A `[[ -o ]]` name this shell does not have is a quiet false, which
		// is not what the same name does to `set -o` one line above.
		{"UnknownConditionOptionIsAStatus", s.UnknownConditionOptionIsAStatus, interp.No},
		// `set -h` is command tracking here, and `set -m` is granted to a
		// script with no terminal — measured, silently.
		{"SetHasTheHLetter", s.SetHasTheHLetter, interp.Yes},
		{"SetHLetterTracksCommands", s.SetHLetterTracksCommands, interp.Yes},
		{"MonitorNeedsATerminal", s.MonitorNeedsATerminal, interp.No},
		{"ReturnOutsideAFunctionIsRefused", s.ReturnOutsideAFunctionIsRefused, interp.Yes},
		// And the neighboring question, which is about a `return` this shell
		// *does* obey: one at the top of a startup file. bash discards the
		// argument there — measured through a pty, an rc of `return 3` leaves
		// `$?` as 0 at the first prompt and `false; return 3` leaves 1, while
		// `(exit 5)` as the last line leaves 5. So the file's status carries
		// out and the number on the `return` does not (#1422).
		{"StartupFileReturnCarriesItsArgument", s.StartupFileReturnCarriesItsArgument, interp.No},
		{"LoneDashIsAnOption", s.LoneDashIsAnOption, interp.No},
		// The one shell in the panel that survives a failed expansion: it
		// gives up the line and runs the next one, where the other three end
		// the shell. Measured over both routes and both separators (#1171).
		{"FailedExpansionAbandonsTheLine", s.FailedExpansionAbandonsTheLine, interp.Yes},
		// One line for a bad option word here, and the reason is the
		// fatality rather than a rule: the loop never reaches the second
		// word. Pinned so that staying on this answer is a decision (#1170).
		{"SetReportsEveryBadOption", s.SetReportsEveryBadOption, interp.No},
		{"ReadonlyReassignmentByDeclarationFatal", s.ReadonlyReassignmentByDeclarationFatal, interp.No},
		{"UnsetFunctionChecksTheName", s.UnsetFunctionChecksTheName, interp.No},
		{"UnsetFunctionReportsMissing", s.UnsetFunctionReportsMissing, interp.No},
		// What `type` does: whether it follows the sentence with the
		// function itself, and whether `--` ends its options.
		{"TypePrintsFunctionBody", s.TypePrintsFunctionBody, interp.Yes},
		{"TypeEndsOptionsWithDashDash", s.TypeEndsOptionsWithDashDash, interp.Yes},
		{"TypeNamesTheKindWithDashT", s.TypeNamesTheKindWithDashT, interp.Yes},
		{"JobsShowBackgroundCommand", s.JobsShowBackgroundCommand, interp.Yes},
		{"JobsListNewestFirst", s.JobsListNewestFirst, interp.No},
		{"JobsListFinishedJobs", s.JobsListFinishedJobs, interp.Yes},
		// All three pseudo-conditions, RETURN being this shell's alone —
		// and none of them follows the script into a call or a subshell
		// it was not set in.
		{"TrapHasErrCondition", s.TrapHasErrCondition, interp.Yes},
		{"TrapHasDebugCondition", s.TrapHasDebugCondition, interp.Yes},
		{"TrapHasReturnCondition", s.TrapHasReturnCondition, interp.Yes},
		{"ErrTrapRunsInsideFunctions", s.ErrTrapRunsInsideFunctions, interp.No},
		{"ErrTrapRunsInSubshells", s.ErrTrapRunsInSubshells, interp.No},
		{"DebugTrapRunsInsideCalls", s.DebugTrapRunsInsideCalls, interp.No},
		{"DebugTrapRunsInSubshells", s.DebugTrapRunsInSubshells, interp.No},
		// The parent's trap listing survives every boundary but a process
		// substitution, EXIT trap included, and an inherited ignore is
		// listed like any other trap.
		{"SubshellKeepsTrapListing", s.SubshellKeepsTrapListing, interp.Yes},
		{"PipelineElementKeepsTrapListing", s.PipelineElementKeepsTrapListing, interp.Yes},
		{"BackgroundJobKeepsTrapListing", s.BackgroundJobKeepsTrapListing, interp.Yes},
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
	if got, want := bash.Diagnostics().TypeKeyword, "%[1]s is a shell keyword"; got != want {
		t.Errorf("TypeKeyword = %q, want %q", got, want)
	}
	if got, want := bash.Diagnostics().TypeFunction, "%[1]s is a function"; got != want {
		t.Errorf("TypeFunction = %q, want %q", got, want)
	}
	if got, want := bash.Diagnostics().TypeNotFound, "type: %[1]s: not found"; got != want {
		t.Errorf("TypeNotFound = %q, want %q", got, want)
	}
	// A complaint rather than a report, so it goes to standard error and
	// `type nope 2>/dev/null` says nothing at all. Half the panel disagrees.
	if bash.Diagnostics().TypeNotFoundOnStdout {
		t.Error("TypeNotFoundOnStdout = true, want the line on standard error")
	}
	if got, want := bash.Diagnostics().SyntaxStatus(), 2; got != want {
		t.Errorf("syntax-error status = %d, want %d", got, want)
	}
	// A compound command's failed open is reported at the redirect's own
	// line here: a `while` loop opening on line 2 with `done < missing` on
	// line 5 says 5. A *simple* command is the command's line in every
	// dialect, so this answers only about the compound case.
	if got, want := bash.Diagnostics().RedirectFailureLine, interp.LineOfRedirect; got != want {
		t.Errorf("RedirectFailureLine = %v, want %v", got, want)
	}
}

// TestDerivesFromTheStandardNotFromASibling is the property the package
// comment promises. A preset that inherits from another shell inherits its
// future mistakes; this one starts from POSIX and overrides only what was
// measured.
func TestDerivesFromTheStandardNotFromASibling(t *testing.T) {
	posix := interp.PosixSemantics()
	s := bash.Semantics()
	if s == posix {
		t.Error("the preset overrides nothing, which cannot be right")
	}
}

// TestUnterminatedNamesTheConstructAndItsLine is bash's view of input that ran
// out: the construct and where it began, and nothing about what would have
// closed it. The other three name three other parts of the same state.
func TestUnterminatedNamesTheConstructAndItsLine(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"if", "syntax error: unexpected end of file from `if' command on line 1"},
		{"if true; then echo x", "syntax error: unexpected end of file from `if' command on line 1"},
		{"for i in a; do echo x", "syntax error: unexpected end of file from `for' command on line 1"},
		{"{ echo x", "syntax error: unexpected end of file from `{' command on line 1"},
	} {
		_, err := syntax.Parse(tc.src, bash.Dialect())
		if got := bash.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestUnterminatedWithNothingOpenSaysSoAndStopsThere: `f()` given no body at
// all runs out of input with the parens already closed, so there is no
// construct to name — measured, bash prints the diagnosis and nothing after
// it, rather than the same sentence with an empty construct in it.
func TestUnterminatedWithNothingOpenSaysSoAndStopsThere(t *testing.T) {
	for _, src := range []string{"f()", "function f"} {
		_, err := syntax.Parse(src, bash.Dialect())
		want := "syntax error: unexpected end of file"
		if got := bash.Diagnostics().ParseFailure(err); got != want {
			t.Errorf("%q: got %q, want %q", src, got, want)
		}
	}
}

// TestAFunctionBodyThatIsNotCompoundIsAnUnexpectedToken: bash alone refuses a
// simple command as a body, and words the refusal as a token in the wrong
// place — which is what earns it the echoed second line, and what names the
// word, the assignment or the redirection operator the body began with.
func TestAFunctionBodyThatIsNotCompoundIsAnUnexpectedToken(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"f() echo hi; f", "syntax error near unexpected token `echo'"},
		{"f() x=1; f", "syntax error near unexpected token `x=1'"},
		{"f() >out; f", "syntax error near unexpected token `>'"},
		{"f() ;", "syntax error near unexpected token `;'"},
	} {
		_, err := syntax.Parse(tc.src, bash.Dialect())
		if got := bash.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
		// And the whole report, which is the part that was missing: the
		// message, then the source line quoted back.
		full := bash.Diagnostics().ParseDiagnostic("bash", "-c", err, tc.src)
		if n := strings.Count(full, "\n"); n != 2 {
			t.Errorf("%q: report = %q, want two lines", tc.src, full)
		}
		if !strings.Contains(full, "`"+tc.src+"'") {
			t.Errorf("%q: report = %q, want the offending line echoed", tc.src, full)
		}
	}
}

// TestBorrowedTextIsNamedTwoWays is the reason the naming is two fields.
//
// bash puts a sourced file's path where its own name goes and labels `eval`
// after it, so one shell answers the same question differently depending on
// which kind of borrowed text failed.
func TestBorrowedTextIsNamedTwoWays(t *testing.T) {
	d := bash.Diagnostics()
	if got, want := d.SourceFileNaming, interp.SourceReplacesShell; got != want {
		t.Errorf("a sourced file: got %v, want it to replace the shell's name", got)
	}
	if got, want := d.EvalNaming, interp.SourceBeforeLocation; got != want {
		t.Errorf("eval: got %v, want it named before the location", got)
	}
	// And the end of input is a line further on than the other three put it,
	// which is only visible when the text does not end in a newline.
	if !d.UnterminatedEndsOnNextLine {
		t.Error("the end of input should be the line after the text")
	}
}

// TestArithmeticFailuresNameTheToken is bash's shape, and the only one of the
// four that names the token it blamed.
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

func TestArithmeticFailuresNameTheToken(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"echo $((1 2))", "sh: line 1: " + `1 2: arithmetic syntax error in expression (error token is "2")` + "\n"},
		{"echo $((1+))", "sh: line 1: " + `1+: arithmetic syntax error: operand expected (error token is "+")` + "\n"},
	} {
		if got := arithRun(t, tc.src); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestUnexpectedTokensAreNamedPlainly is bash's shape, which never says what
// it wanted instead.
func TestUnexpectedTokensAreNamedPlainly(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"echo )", "syntax error near unexpected token `)'"},
		{"for i in a b; echo $i; done", "syntax error near unexpected token `echo'"},
	} {
		_, err := syntax.Parse(tc.src, bash.Dialect())
		if got := bash.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestAParenAfterAWordIsAFunctionDefinition is bash committing at the paren,
// which is why `f ( x )` blames the word and not the paren.
func TestAParenAfterAWordIsAFunctionDefinition(t *testing.T) {
	_, err := syntax.Parse("f ( x )", bash.Dialect())
	want := "syntax error near unexpected token `x'"
	if got := bash.Diagnostics().ParseFailure(err); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestPrintfAnswers covers bash's four, one of which is bash's alone.
func TestPrintfAnswers(t *testing.T) {
	s := bash.Semantics()
	for _, tc := range []struct {
		axis string
		got  any
		want any
	}{
		{"PrintfReportsBadNumber", s.PrintfReportsBadNumber, interp.Yes},
		// bash alone: an operand present and empty is an error, where a
		// missing one is an error in none of the four.
		{"PrintfEmptyIsNotANumber", s.PrintfEmptyIsNotANumber, interp.Yes},
		{"PrintfBackslashC", s.PrintfBackslashC, interp.PrintfBackslashCLiteral},
		{"PrintfQuote", s.PrintfQuote, interp.PrintfQuoteBackslash},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.axis, tc.got, tc.want)
		}
	}
}

// TestCdAnswers covers bash's three, one of which zsh answers the other way.
func TestCdAnswers(t *testing.T) {
	s, d := bash.Semantics(), bash.Diagnostics()
	if got, want := s.CdWithoutHomeIsAnError, interp.Yes; got != want {
		t.Errorf("CdWithoutHomeIsAnError = %v, want %v", got, want)
	}
	if got, want := s.CdDashPrintsTheDirectory, interp.Yes; got != want {
		t.Errorf("CdDashPrintsTheDirectory = %v, want %v", got, want)
	}
	if got, want := d.CdCannotChange, "cd: %[1]s: %[2]s"; got != want {
		t.Errorf("CdCannotChange = %q, want %q", got, want)
	}
}

// TestGetoptsAnswers: bash names itself and no line for these two complaints,
// where it gives a line to everything else it says.
func TestGetoptsAnswers(t *testing.T) {
	d := bash.Diagnostics()
	if !d.GetoptsNamesNoLine {
		t.Error("getopts should name the shell without a line")
	}
	if d.GetoptsUnprefixed {
		t.Error("getopts should still name the shell")
	}
	if got, want := bash.Semantics().GetoptsAssignmentRestartsWord, interp.Yes; got != want {
		t.Errorf("GetoptsAssignmentRestartsWord = %v, want %v", got, want)
	}
}

// TestParametersBashProvides: which parameters a shell supplies is the same
// kind of question as which builtins it has, so it is answered through the
// same seam rather than as an axis.
func TestParametersBashProvides(t *testing.T) {
	r := &interp.Runner{Dialect: presetDialect()}
	bash.Apply(r)
	for _, name := range []string{"UID", "EUID", "RANDOM", "SECONDS"} {
		f, err := syntax.Parse(`[ -n "${`+name+`-}" ] && echo have`, bash.Dialect())
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		rr := &interp.Runner{Stdout: &out, Dialect: presetDialect()}
		bash.Apply(rr)
		if _, err := rr.Run(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(out.String()) != "have" {
			t.Errorf("%s: got %q, want bash to provide it", name, out.String())
		}
	}
}

// TestBothDeclarationNames: bash spells the declaration two ways, and the
// assignment rule follows the second name as well as the first.
func TestBothDeclarationNames(t *testing.T) {
	// A file the unprotected word would match, so the assertion distinguishes
	// the two paths. Without one, `declare x=*` matches nothing and comes
	// back as `x=*` whether or not the value was ever protected — which is a
	// test that passes against the bug.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x=a"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"typeset", "declare"} {
		f, err := syntax.Parse(name+` x=*; echo "$x"`, bash.Dialect())
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		s, d := bash.Semantics(), bash.Diagnostics()
		r := &interp.Runner{Stdout: &out, Stderr: &out, Semantics: &s, Diagnostics: &d, Dir: dir, Dialect: presetDialect()}
		bash.Apply(r)
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		// The character, not the file it would have matched: the assignment
		// rule follows the second name as well as the first.
		if strings.TrimSpace(out.String()) != "*" {
			t.Errorf("%s: got %q, want the pattern left alone", name, out.String())
		}
	}
}

// TestSelectMenuLayout: the widest presentation difference in the panel. bash
// prints one item per line while the list would fit on one line and switches
// to tab-separated columns once it would not, which is the opposite way round
// from how it sounds.
func TestSelectMenuLayout(t *testing.T) {
	if got, want := bash.Semantics().SelectLayout, interp.SelectMenuVerticalThenColumns; got != want {
		t.Errorf("SelectLayout = %v, want %v", got, want)
	}
	if got, want := bash.Diagnostics().SelectPrompt, "#? "; got != want {
		t.Errorf("SelectPrompt = %q, want %q", got, want)
	}
}

// TestPipelineStatusName: the core keeps the record of what a pipeline's
// elements reported and a dialect names it. bash's name is the uppercase one.
func TestPipelineStatusName(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`false | true | false; echo "[${PIPESTATUS[@]}]"`, "[1 0 1]"},
		// And not under any other name, which is what makes it an answer.
		{`false | true | false; echo "[${pipestatus[@]}]"`, "[]"},
	} {
		f, err := syntax.Parse(tc.src, bash.Dialect())
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		s, d := bash.Semantics(), bash.Diagnostics()
		r := &interp.Runner{Stdout: &out, Stderr: &out, Semantics: &s, Diagnostics: &d, Dialect: presetDialect()}
		bash.Apply(r)
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(out.String()) != tc.want {
			t.Errorf("%s: got %q, want %q", tc.src, out.String(), tc.want)
		}
	}
}

// TestRegexMatchName: the core records what `=~` captures and a dialect names
// it. bash's name is BASH_REMATCH: the whole match at 0, then the groups —
// and a failed match empties it rather than leaving the capture before last.
func TestRegexMatchName(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`[[ abcd =~ (b)(c) ]]; echo "[${BASH_REMATCH[0]}|${BASH_REMATCH[1]}|${BASH_REMATCH[2]}]"`, "[bc|b|c]"},
		{`[[ ab =~ a ]]; [[ ab =~ q ]]; echo "n=${#BASH_REMATCH[@]}"`, "n=0"},
	} {
		f, err := syntax.Parse(tc.src, bash.Dialect())
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		s, d := bash.Semantics(), bash.Diagnostics()
		r := &interp.Runner{Stdout: &out, Stderr: &out, Semantics: &s, Diagnostics: &d, Dialect: presetDialect()}
		bash.Apply(r)
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(out.String()) != tc.want {
			t.Errorf("%s: got %q, want %q", tc.src, out.String(), tc.want)
		}
	}
}

// TestAParseFailureNamesItsOriginAndEchoesTheLine: bash decorates a parse
// failure twice over, and neither part appears on a runtime diagnostic about
// the same input.
func TestAParseFailureNamesItsOriginAndEchoesTheLine(t *testing.T) {
	d := bash.Diagnostics()
	if !d.NamesTheInputInLocation {
		t.Error("bash writes `bash: -c: line 1:` for a parse failure")
	}
	if !d.EchoesTheOffendingLine {
		t.Error("bash repeats the offending source line after the message")
	}
	// A script names itself instead, so the origin is not named twice.
	if bash.Diagnostics().ForScript().NamesTheInputInLocation != d.NamesTheInputInLocation {
		t.Error("the script form should answer this the same way; the front end supplies no origin for a file")
	}
}

// TestAMalformedExpressionIsAFailedCommand: bash finds a bad expression while
// expanding, so it reports the status of a command that failed rather than of
// a script that would not parse — and carries none of a parse failure's
// decoration. We find it while parsing, which is why it has to be said.
func TestAMalformedExpressionIsAFailedCommand(t *testing.T) {
	if got, want := bash.Diagnostics().ArithFailureStatus, 1; got != want {
		t.Errorf("ArithFailureStatus = %d, want %d", got, want)
	}
	// And it really is a failed command now: the file reads, the command
	// runs, and the whole of what is written is one line with no origin
	// named and no echoed source (#865).
	//
	// Fatal, and measured rather than reasoned: a failed arithmetic
	// *expansion* ends the script in bash, ksh93 and zsh alike, so the
	// `echo` after it never runs. That is the expansion's answer and not the
	// arithmetic command's — `((1 2)); echo st=$?` reaches its echo.
	out, st := answersRun(t, `echo $((1 2)); echo "after=$?"`)
	want := "sh: line 1: " + `1 2: arithmetic syntax error in expression (error token is "2")` + "\n"
	if out != want || st != 1 {
		t.Errorf("got %q at %d, want %q at 1", out, st, want)
	}
}

// TestPatternGroups: which groups this shell reads, and where.
func TestPatternGroups(t *testing.T) {
	if bash.Dialect().ExtendedPattern {
		t.Error("bash does not read extended patterns in a case pattern")
	}
	if !bash.Dialect().ExtendedPatternInCondition {
		t.Error("bash reads them inside `[[ ]]`")
	}
}

// The notice a signal that ended a command gets, which this shell says with
// all three of the things there are to say about one.
func TestAKilledCommandIsSaidBackInFull(t *testing.T) {
	dg := bash.Diagnostics()
	if got, want := dg.KilledCommandNotice, "%5[1]d %-27[2]s%[3]s"; got != want {
		t.Errorf("KilledCommandNotice = %q, want %q", got, want)
	}
	// The same twenty-seven-column field the `jobs` listing uses, which is
	// why the two line up under each other.
	if !strings.Contains(dg.JobLine, "%-27") {
		t.Errorf("JobLine = %q, want it to use the same column as the notice", dg.JobLine)
	}
	if dg.KilledCommandNoticeUnprefixed {
		t.Error("the notice is prefixed here, like every other message this shell prints")
	}
	if dg.SignalDescriptions != nil {
		t.Error("this shell takes the words for a signal from the machine")
	}
}

// A substitution's body is numbered from the file, whichever way it is
// written.
func TestASubstitutionsBodyIsNumberedFromTheFile(t *testing.T) {
	if bash.Diagnostics().BackquotedSubstitutionRestartsLines {
		t.Error("backquotes are numbered from the file here, like $( )")
	}
}

// Which `set -o` names a shell has is the same kind of question as which
// parameters it supplies, so it goes through the same seam. This one has the
// most, and five belong to it alone.
//
// "Has the name" is not the same as "succeeds": a name this shell has but
// does not implement is refused as unimplemented, and only a name it does
// not have at all is an invalid name. That difference is the whole point of
// the change, so it is what is asserted.
func TestSetOptionNamesBashHas(t *testing.T) {
	for _, c := range []struct {
		name string
		has  bool
	}{
		// Its own, and the reason this change exists: `set +o posix` is the
		// thirteenth line of Homebrew's `brew`.
		{"posix", true},
		{"errtrace", true},
		{"functrace", true},
		{"history", true},
		{"interactive-comments", true},
		// Shared with some but not all of the panel.
		{"braceexpand", true},
		{"hashall", true},
		{"privileged", true},
		// Real here, and in the listings because it is declared.
		{"pipefail", true},
		// Common to all four, so it needs no declaring and must still work.
		{"noexec", true},
		// Other shells' names for what this one spells its own way.
		{"trackall", false},
		{"histignoredups", false},
		// Not an option anywhere.
		{"bogusname", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			sem := bash.Semantics()
			dg := bash.Diagnostics()
			var out bytes.Buffer
			r := &interp.Runner{Stdout: &out, Stderr: &out, Semantics: &sem, Diagnostics: &dg, Dialect: presetDialect()}
			bash.Apply(r)
			f, err := syntax.Parse("set +o "+c.name, bash.Dialect())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if has := !strings.Contains(out.String(), "invalid option name"); has != c.has {
				t.Errorf("set +o %s said %q; has the name = %v, want %v",
					c.name, out.String(), has, c.has)
			}
		})
	}
}

// The one signal this shell writes bare, with neither the location nor the
// process id. Reproduced because the dialect is 5.3; 3.2 writes the prefix.
func TestTerminateIsWrittenBare(t *testing.T) {
	if got, want := bash.Diagnostics().KilledCommandNoticeBareForTerminate, "%-27[1]s%[2]s"; got != want {
		t.Errorf("KilledCommandNoticeBareForTerminate = %q, want %q", got, want)
	}
}

// The status this shell answers a failed expansion with when the program came
// from an argument rather than from a file.
func TestAFailedExpansionFromACommandString(t *testing.T) {
	if got, want := bash.Diagnostics().ExpansionFailureStatusFromCommandString, 127; got != want {
		t.Errorf("ExpansionFailureStatusFromCommandString = %d, want %d", got, want)
	}
}

// TestDeclareReportsARefusedName covers the third declaration spelling, which
// only two dialects have: `declare r=2` against a readonly name returns 1
// where the builtin's own success would have said nothing went wrong.
func TestDeclareReportsARefusedName(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), "readonly r=1\ndeclare r=2\necho st=$?\n")
	if !strings.Contains(out, "st=1\n") {
		t.Errorf("output = %q, want the refusal to fail the builtin", out)
	}
}

// The letters `$-` starts with, measured identical under -c, a script file
// and standard input; the letters that describe the route come from
// Runner.Route and are asserted below.
func TestDollarDashStartupLetters(t *testing.T) {
	if got, want := bash.Semantics().DefaultOptionLetters, "hB"; got != want {
		t.Errorf("DefaultOptionLetters = %q, want %q", got, want)
	}
}

// The two route axes. Measured on bash 5.3.15, 2026-09-05: `bash -c 'echo
// $-'` reports `hBc` — the `c` and not the `s`, where ksh93 shows both and
// dash and zsh show neither.
//
// `s` for the standard-input route is unanimous and has no axis, so there is
// nothing here to assert about it. bash 3.2 is the one shell that dissents on
// it and it dissents against bash 5.3 rather than against the panel — it
// shows the letter only where `-s` was written. Recorded in
// docs/spec/invocation.md; this dialect follows 5.3, as it does everywhere.
func TestDollarDashRouteLetters(t *testing.T) {
	s := bash.Semantics()
	if got := s.CommandStringShowsCInDollarDash; got != interp.Yes {
		t.Errorf("CommandStringShowsCInDollarDash = %v, want Yes", got)
	}
	if got := s.CommandStringShowsSInDollarDash; got != interp.No {
		t.Errorf("CommandStringShowsSInDollarDash = %v, want No", got)
	}
}

// TestDollarSingleAnswers covers the three `$'…'` axes.
//
// The `\c` answer is the one that separates bash from ksh93, and only away
// from the letters: both uppercase first and then agree over `@` through `_`,
// so `$'\cA'` is 0x01 either way and `$'\c1'` is 0x11 here and `q` there.
func TestDollarSingleAnswers(t *testing.T) {
	s := bash.Semantics()
	for _, tc := range []struct {
		axis string
		got  any
		want any
	}{
		{"DollarSingleBackslashC", s.DollarSingleBackslashC, interp.DollarSingleControlMasked},
		{"DollarSingleUnknownEscape", s.DollarSingleUnknownEscape, interp.DollarSingleUnknownKeepsBackslash},
		// C-string semantics: `$'a\0b'` is `a`, and the length of what was
		// assigned is 1.
		{"DollarSingleNulTruncates", s.DollarSingleNulTruncates, interp.Yes},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.axis, tc.got, tc.want)
		}
	}
}

// The panel's holdout on the login profile. Measured 2026-09-05 with a scratch
// HOME: `exec -a -bash bash script.sh` reads neither ~/.bash_profile nor
// /etc/profile, and the same is true of `-c`, of a program on standard input
// and of `-s` — where dash, ksh93 and zsh all read theirs on all four routes.
// bash 3.2 and bash invoked as `sh` answer the same way.
//
// Not because it declines to be a login shell: `shopt login_shell` is on. It
// is that a non-interactive bash reads no startup file unless `--login` was
// written out, and this front end has no `--login` to write (#482).
func TestBashReadsNoProfileWithAScriptToRun(t *testing.T) {
	if bash.Semantics().LoginProfileWhenNonInteractive {
		t.Error("LoginProfileWhenNonInteractive = true, want false")
	}
	if !interp.PosixSemantics().LoginProfileWhenNonInteractive {
		t.Error("the POSIX preset no longer reads it, so this preset overrides nothing")
	}
}

// TestDollarDashInteractiveStartupLetters. Measured 2026-09-05 on bash 5.3.15
// with a scratch HOME: `bash -i script.sh` reports `hiBH`, so the interactive
// set adds the history-expansion letter and keeps the rest. `i` is not in the
// field — it is unanimous and comes from the runner — and neither is `m`: this
// shell's monitor is off there, which `set -o` confirms.
//
// bash 3.2 answers `hiB` for the same invocation and `hiBHc` for `-i -c`, so
// it disagrees with itself as well as with 5.3. 5.3 is the panel member that
// counts.
func TestDollarDashInteractiveStartupLetters(t *testing.T) {
	if got, want := bash.Semantics().InteractiveOptionLetters, "hBH"; got != want {
		t.Errorf("InteractiveOptionLetters = %q, want %q", got, want)
	}
}

// The startup remark, which is the whole of what bash reproduces here.
//
// Measured 2026-09-05 with no terminal on any of the three standard streams:
// `bash: no job control in this shell` in 5.3.15 and 3.2.57, and `sh: no job
// control in this shell` in 3.2 run as `sh` — its own name, on `-i script.sh`
// as well as on `-i -c` and `-i -s`, and never the script's.
//
// 5.3.15 writes one line more, above it: `bash: cannot set terminal process
// group (11143): Inappropriate ioctl for device`. It is deliberately not
// reproduced, and the reason that settles it is that the other two members do
// not write it at all — it is one version's extra line and not bash's
// wording. See interp.Diagnostics.NoJobControlAtStartup.
func TestTheStartupRemarkAboutJobControl(t *testing.T) {
	if got, want := bash.Diagnostics().NoJobControlAtStartup, "no job control in this shell"; got != want {
		t.Errorf("NoJobControlAtStartup = %q, want %q", got, want)
	}
	if bash.Diagnostics().NoJobControlAtStartupNamesTheScript {
		t.Error("NoJobControlAtStartupNamesTheScript = true, want false — bash names itself, not the script")
	}
}

// The shared wording and the shared status, both by omission: bash is what the
// defaults were written from. Measured, `jobs %9` says `jobs: %9: no such job`
// and reports 1 in all three members.
func TestTheJobSpecThatNamesNothing(t *testing.T) {
	if got := bash.Diagnostics().NoSuchJob; got != "" {
		t.Errorf("NoSuchJob = %q, want empty — the shared wording", got)
	}
	if got := bash.Diagnostics().NoSuchJobStatus; got != 0 {
		t.Errorf("NoSuchJobStatus = %d, want 0 — the shared 1", got)
	}
}

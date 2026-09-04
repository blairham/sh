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
	// Does not expand them in a script; the prompt is a different
	// question and the front end answers it.
	if got, want := bash.Dialect().ExpandAliases, false; got != want {
		t.Errorf("ExpandAliases = %v, want %v", got, want)
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
		{"AssignmentUpdatesPipelineStatus", s.AssignmentUpdatesPipelineStatus, interp.Yes},
		{"UnsetEndsTheProducedPipelineStatus", s.UnsetEndsTheProducedPipelineStatus, interp.No},
		{"ArrayScalarIsTheWholeArray", s.ArrayScalarIsTheWholeArray, interp.No},
		{"SelectPromptNeedsTerminal", s.SelectPromptNeedsTerminal, interp.No},
		{"SelectEofIsSuccess", s.SelectEofIsSuccess, interp.No},
		{"SelectTakesUnterminatedReply", s.SelectTakesUnterminatedReply, interp.No},
		{"SelectEofPrintsNewline", s.SelectEofPrintsNewline, interp.Yes},
		{"SelectEofEndsPromptLine", s.SelectEofEndsPromptLine, interp.No},
		{"SelectAssumesUnboundedWidth", s.SelectAssumesUnboundedWidth, interp.No},
		{"DeclaredNameWithoutValueIsEmpty", s.DeclaredNameWithoutValueIsEmpty, interp.No},
		{"TypesetLocalNeedsKeywordFunction", s.TypesetLocalNeedsKeywordFunction, interp.No},
		{"BraceExpansion", s.BraceExpansion, interp.Yes},
		{"IndirectionYieldsName", s.IndirectionYieldsName, interp.No},
		{"ReadonlyReassignmentFatal", s.ReadonlyReassignmentFatal, interp.No},
		{"RegexQuotingMakesLiteral", s.RegexQuotingMakesLiteral, interp.Yes},
		{"ArithLeadingZeroIsOctal", s.ArithLeadingZeroIsOctal, interp.Yes},
		// A `jobs` listing: which end it starts from, and whether a job that
		// has already ended appears in it at all. Both split the panel two
		// and two, which is why both are fields.
		// Whether an unassigned subscript is an element.
		{"ArraysAreSparse", s.ArraysAreSparse, interp.Yes},
		{"AnnouncesBackgroundJob", s.AnnouncesBackgroundJob, interp.Yes},
		{"ReportsACommandKilledBySignal", s.ReportsACommandKilledBySignal, interp.Yes},
		{"ReportsAnyKilledPipelineElement", s.ReportsAnyKilledPipelineElement, interp.No},
		{"ChildInterruptEndsTheScript", s.ChildInterruptEndsTheScript, interp.No},
		{"CdRefusesUnknownOption", s.CdRefusesUnknownOption, interp.Yes},
		{"CdLastPathOptionWins", s.CdLastPathOptionWins, interp.Yes},
		{"BadSetOptionNameFatal", s.BadSetOptionNameFatal, interp.No},
		{"ReturnOutsideAFunctionIsRefused", s.ReturnOutsideAFunctionIsRefused, interp.Yes},
		{"LoneDashIsAnOption", s.LoneDashIsAnOption, interp.No},
		{"ReadonlyReassignmentFatalFromCommandString", s.ReadonlyReassignmentFatalFromCommandString, interp.Yes},
		{"ReadonlyReassignmentByDeclarationFatal", s.ReadonlyReassignmentByDeclarationFatal, interp.No},
		{"UnsetFunctionChecksTheName", s.UnsetFunctionChecksTheName, interp.No},
		{"UnsetFunctionReportsMissing", s.UnsetFunctionReportsMissing, interp.No},
		// What `type` does: whether it follows the sentence with the
		// function itself, and whether `--` ends its options.
		{"TypePrintsFunctionBody", s.TypePrintsFunctionBody, interp.Yes},
		{"TypeEndsOptionsWithDashDash", s.TypeEndsOptionsWithDashDash, interp.Yes},
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
func TestArithmeticFailuresNameTheToken(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"echo $((1 2))", `1 2: arithmetic syntax error in expression (error token is "2")`},
		{"echo $((1+))", `1+: arithmetic syntax error: operand expected (error token is "+")`},
	} {
		_, err := syntax.Parse(tc.src, bash.Dialect())
		if got := bash.Diagnostics().ParseFailure(err); got != tc.want {
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
	r := &interp.Runner{}
	bash.Apply(r)
	for _, name := range []string{"UID", "EUID", "RANDOM", "SECONDS"} {
		f, err := syntax.Parse(`[ -n "${`+name+`-}" ] && echo have`, bash.Dialect())
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		rr := &interp.Runner{Stdout: &out}
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
		r := &interp.Runner{Stdout: &out, Stderr: &out, Semantics: &s, Diagnostics: &d, Dir: dir}
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
		r := &interp.Runner{Stdout: &out, Stderr: &out, Semantics: &s, Diagnostics: &d}
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
		r := &interp.Runner{Stdout: &out, Stderr: &out, Semantics: &s, Diagnostics: &d}
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
	f, err := syntax.Parse(`echo $((1 2))`, bash.Dialect())
	if err == nil {
		t.Fatal("want a parse failure")
	}
	_ = f
	d := bash.Diagnostics()
	if got, want := d.StatusForParseError(err), 1; got != want {
		t.Errorf("status = %d, want %d — a failed command, not a failed parse", got, want)
	}
	// And none of the decoration: no named origin, no echoed line.
	out := d.ParseDiagnostic("bash", "-c", err, "echo $((1 2))")
	if strings.Contains(out, "-c") {
		t.Errorf("got %q, want no origin named", out)
	}
	if strings.Count(out, "\n") != 1 {
		t.Errorf("got %q, want no echoed line", out)
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
		// Common to all four, so it needs no declaring and must still work.
		{"noexec", true},
		// Not an option anywhere.
		{"bogusname", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			sem := bash.Semantics()
			dg := bash.Diagnostics()
			var out bytes.Buffer
			r := &interp.Runner{Stdout: &out, Stderr: &out, Semantics: &sem, Diagnostics: &dg}
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
	if got, want := bash.Diagnostics().UnsetParameterStatusFromCommandString, 127; got != want {
		t.Errorf("UnsetParameterStatusFromCommandString = %d, want %d", got, want)
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

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
		{"AnnouncesBackgroundJob", s.AnnouncesBackgroundJob, interp.Yes},
		{"JobsShowBackgroundCommand", s.JobsShowBackgroundCommand, interp.Yes},
		{"JobsListNewestFirst", s.JobsListNewestFirst, interp.No},
		{"JobsListFinishedJobs", s.JobsListFinishedJobs, interp.Yes},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.axis, tc.got, tc.want)
		}
	}
}

func TestDiagnostics(t *testing.T) {
	if got, want := bash.Diagnostics().SyntaxStatus(), 2; got != want {
		t.Errorf("syntax-error status = %d, want %d", got, want)
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

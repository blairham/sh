// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"context"
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
		{"DeclaredNameWithoutValueIsEmpty", s.DeclaredNameWithoutValueIsEmpty, interp.No},
		{"TypesetLocalNeedsKeywordFunction", s.TypesetLocalNeedsKeywordFunction, interp.No},
		{"BraceExpansion", s.BraceExpansion, interp.Yes},
		{"IndirectionYieldsName", s.IndirectionYieldsName, interp.No},
		{"ReadonlyReassignmentFatal", s.ReadonlyReassignmentFatal, interp.No},
		{"RegexQuotingMakesLiteral", s.RegexQuotingMakesLiteral, interp.Yes},
		{"ArithLeadingZeroIsOctal", s.ArithLeadingZeroIsOctal, interp.Yes},
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
	for _, name := range []string{"typeset", "declare"} {
		f, err := syntax.Parse(name+` -i n=3*3; echo "$n"`, bash.Dialect())
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
		// 9 rather than the pattern: the value is an assignment's and is
		// neither globbed nor split, and the integer attribute evaluates it.
		if strings.TrimSpace(out.String()) != "9" {
			t.Errorf("%s: got %q, want 9", name, out.String())
		}
	}
}

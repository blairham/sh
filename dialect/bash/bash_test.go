// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
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

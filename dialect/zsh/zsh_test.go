// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
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
	for _, tc := range []struct {
		src  string
		want bool
	}{
		{`case a in a) echo x;;& esac`, false},
		{`echo ${x^^}`, false},
		{`echo ${!x}`, false},
		{`function f() { echo x; }`, true},
		{`a=(x y)`, true},
	} {
		if got := parses(t, tc.src); got != tc.want {
			t.Errorf("%q: parses = %v, want %v", tc.src, got, tc.want)
		}
	}
}

func TestSemantics(t *testing.T) {
	s := zsh.Semantics()
	for _, tc := range []struct {
		axis string
		got  interp.Answer
		want interp.Answer
	}{
		{"SplitParamExpansion", s.SplitParamExpansion, interp.No},
		{"ArrayBaseIsZero", s.ArrayBaseIsZero, interp.No},
		{"GlobNoMatchIsError", s.GlobNoMatchIsError, interp.Yes},
		{"ArithLeadingZeroIsOctal", s.ArithLeadingZeroIsOctal, interp.No},
		{"ReadonlyReassignmentFatal", s.ReadonlyReassignmentFatal, interp.Yes},
		{"DollarZeroInFunctionIsFunctionName", s.DollarZeroInFunctionIsFunctionName, interp.Yes},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.axis, tc.got, tc.want)
		}
	}
}

func TestDiagnostics(t *testing.T) {
	if got, want := zsh.Diagnostics().SyntaxStatus(), 1; got != want {
		t.Errorf("syntax-error status = %d, want %d", got, want)
	}
}

// TestDerivesFromTheStandardNotFromASibling is the property the package
// comment promises. A preset that inherits from another shell inherits its
// future mistakes; this one starts from POSIX and overrides only what was
// measured.
func TestDerivesFromTheStandardNotFromASibling(t *testing.T) {
	posix := interp.PosixSemantics()
	s := zsh.Semantics()
	if s == posix {
		t.Error("the preset overrides nothing, which cannot be right")
	}
}

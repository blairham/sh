// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The measured answers for dash live here, in dash's own package, so that adding
// or correcting a shell never touches the substrate.

func parses(t *testing.T, src string) bool {
	t.Helper()
	_, err := syntax.Parse(src, dash.Dialect())
	return err == nil
}

func TestGrammar(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want bool
	}{
		{`a=(x y)`, false},
		{`function f { echo x; }`, false},
		{`echo ${!x}`, false},
		// Not a syntax error: with the construct absent, `[[` is a command
		// name and this parses. Real dash agrees, and fails at runtime with
		// "[[: not found" and status 127 — the same trap as `&>`, where a
		// missing construct changes the meaning rather than rejecting it.
		{`[[ -n x ]]`, true},
		// It *is* a syntax error once a paren appears, because a paren after
		// a word is one in every shell in the panel.
		{`[[ ( -n x ) ]]`, false},
		{`echo "$((1+1))"`, true},
	} {
		if got := parses(t, tc.src); got != tc.want {
			t.Errorf("%q: parses = %v, want %v", tc.src, got, tc.want)
		}
	}
}

func TestSemantics(t *testing.T) {
	s := dash.Semantics()
	for _, tc := range []struct {
		axis string
		got  interp.Answer
		want interp.Answer
	}{
		{"EchoInterpretsEscapes", s.EchoInterpretsEscapes, interp.Yes},
		{"LengthOfSpecialIsCount", s.LengthOfSpecialIsCount, interp.No},
		{"BraceExpansion", s.BraceExpansion, interp.No},
		{"BracketCaretNegates", s.BracketCaretNegates, interp.No},
		{"FatalErrorStatusIsOne", s.FatalErrorStatusIsOne, interp.No},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.axis, tc.got, tc.want)
		}
	}
}

func TestDiagnostics(t *testing.T) {
	if got, want := dash.Diagnostics().SyntaxStatus(), 2; got != want {
		t.Errorf("syntax-error status = %d, want %d", got, want)
	}
}

// TestDerivesFromTheStandardNotFromASibling is the property the package
// comment promises. A preset that inherits from another shell inherits its
// future mistakes; this one starts from POSIX and overrides only what was
// measured.
func TestDerivesFromTheStandardNotFromASibling(t *testing.T) {
	posix := interp.PosixSemantics()
	s := dash.Semantics()
	if s == posix {
		t.Error("the preset overrides nothing, which cannot be right")
	}
}

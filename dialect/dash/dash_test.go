// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"bytes"
	"context"
	"strings"
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

// TestSignalNamesHaveNoSIGPrefix is dash's answer where it can be seen.
//
// The prefix is not part of a signal's name here, and dash says so in three
// places with three wordings — including one that names only the first
// character, because that is where it stopped reading.
func TestSignalNamesHaveNoSIGPrefix(t *testing.T) {
	for _, tc := range []struct {
		src    string
		errs   string
		status int
	}{
		// No shell name in front of it: dash prints `trap`'s complaint bare
		// where it prefixes every `kill` diagnostic it has.
		{`trap 'echo x' SIGUSR1`, "trap: SIGUSR1: bad trap\n", 1},
		{`trap 'echo x' USR1`, "", 0},
		{`kill -SIGCONT $$`, "dash: 1: kill: Illegal option -S\n", 2},
		{`kill -s SIGCONT $$`, "dash: 1: kill: invalid signal number or name: SIGCONT\n", 2},
		{`kill -CONT $$`, "", 0},
	} {
		f, err := syntax.Parse(tc.src, dash.Dialect())
		if err != nil {
			t.Fatalf("parse %q: %v", tc.src, err)
		}
		var errs bytes.Buffer
		sem, dg := dash.Semantics(), dash.Diagnostics()
		r := &interp.Runner{Stderr: &errs, Semantics: &sem, Diagnostics: &dg, Name: "dash"}
		st, err := r.Run(context.Background(), f)
		if err != nil {
			t.Fatalf("run %q: %v", tc.src, err)
		}
		if got := errs.String(); got != tc.errs {
			t.Errorf("%s: stderr = %q, want %q", tc.src, got, tc.errs)
		}
		if st != tc.status {
			t.Errorf("%s: status = %d, want %d", tc.src, st, tc.status)
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
		{"SIGPrefixAccepted", s.SIGPrefixAccepted, interp.No},
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

// TestUnterminatedNamesWhatWouldHaveClosedIt is dash's view: the expected word
// and the class of what it found instead, never the construct.
func TestUnterminatedNamesWhatWouldHaveClosedIt(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"if", `Syntax error: end of file unexpected (expecting "then")`},
		{"if true; then echo x", `Syntax error: end of file unexpected (expecting "fi")`},
		{"case a in a) echo x", `Syntax error: end of file unexpected (expecting ";;")`},
		{"{ echo x", `Syntax error: end of file unexpected (expecting "}")`},
	} {
		_, err := syntax.Parse(tc.src, dash.Dialect())
		if got := dash.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestArithmeticFailuresAreDashsOwnShape covers the four dash reaches, and the
// one it does not: a digit too great for its base is not a diagnosis dash has
// — the literal ends there and what follows is left over, so it says the same
// thing it says about `$((1 2))`.
func TestArithmeticFailuresAreDashsOwnShape(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"echo $((1 2))", `arithmetic expression: expecting EOF: "1 2"`},
		{"echo $((1+))", `arithmetic expression: expecting primary: "1+"`},
		{"echo $((1,2))", `arithmetic expression: expecting EOF: "1,2"`},
	} {
		_, err := syntax.Parse(tc.src, dash.Dialect())
		if got := dash.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}

	// `08` parses here and fails when it is evaluated, so it never reaches
	// ParseFailure — and it is the case that says a bad digit is not a
	// diagnosis dash has.
	f, err := syntax.Parse("echo $((08))", dash.Dialect())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var errs bytes.Buffer
	sem, dg := dash.Semantics(), dash.Diagnostics()
	r := &interp.Runner{Stderr: &errs, Semantics: &sem, Diagnostics: &dg, Name: "dash"}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := `arithmetic expression: expecting EOF: "08"`; !strings.Contains(errs.String(), want) {
		t.Errorf("got %q, want it to contain %q", errs.String(), want)
	}
}

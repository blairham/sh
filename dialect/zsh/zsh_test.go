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
		r := &interp.Runner{Stderr: &errs, Semantics: &sem, Diagnostics: &dg, Name: "zsh"}
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
			r := &interp.Runner{Stderr: &errs, Semantics: &sem, Diagnostics: &dg, Name: "zsh"}
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
func TestArithmeticFailuresQuoteNothing(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"echo $((1 2))", "bad math expression: operator expected at `2'"},
		{"echo $((1+))", "bad math expression: operand expected at end of string"},
	} {
		_, err := syntax.Parse(tc.src, zsh.Dialect())
		if got := zsh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
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
		r := &interp.Runner{Stderr: &errs, Semantics: &sem, Diagnostics: &dg, Name: "zsh"}
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatalf("run %q: %v", tc.src, err)
		}
		if got := strings.TrimSpace(errs.String()); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

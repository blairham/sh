// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The measured answers for ksh93 live here, in ksh93's own package, so that adding
// or correcting a shell never touches the substrate.

func parses(t *testing.T, src string) bool {
	t.Helper()
	_, err := syntax.Parse(src, ksh.Dialect())
	return err == nil
}

func TestGrammar(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want bool
	}{
		{`case a in a) echo x;;& esac`, false},
		{`echo ${x^^}`, false},
		{`echo ${!x}`, true},
		{`function f() { echo x; }`, false},
		{`function f { echo x; }`, true},
		{`a=(x y)`, true},
	} {
		if got := parses(t, tc.src); got != tc.want {
			t.Errorf("%q: parses = %v, want %v", tc.src, got, tc.want)
		}
	}
}

func TestSemantics(t *testing.T) {
	s := ksh.Semantics()
	for _, tc := range []struct {
		axis string
		got  interp.Answer
		want interp.Answer
	}{
		{"IndirectionYieldsName", s.IndirectionYieldsName, interp.Yes},
		{"ArithInvalidOctalDigitIsError", s.ArithInvalidOctalDigitIsError, interp.No},
		{"ArithLeadingZeroIsOctal", s.ArithLeadingZeroIsOctal, interp.Yes},
		{"ArithFloat", s.ArithFloat, interp.Yes},
		{"LastPipelineElementInCurrentShell", s.LastPipelineElementInCurrentShell, interp.Yes},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.axis, tc.got, tc.want)
		}
	}
}

func TestDiagnostics(t *testing.T) {
	if got, want := ksh.Diagnostics().SyntaxStatus(), 3; got != want {
		t.Errorf("syntax-error status = %d, want %d", got, want)
	}
}

// TestDerivesFromTheStandardNotFromASibling is the property the package
// comment promises. A preset that inherits from another shell inherits its
// future mistakes; this one starts from POSIX and overrides only what was
// measured.
func TestDerivesFromTheStandardNotFromASibling(t *testing.T) {
	posix := interp.PosixSemantics()
	s := ksh.Semantics()
	if s == posix {
		t.Error("the preset overrides nothing, which cannot be right")
	}
}

// TestApplyRemovesLocal is the one builtin the panel disagrees about. ksh93
// reports it as a command that was not found, which is what removing it from
// the substrate's table produces — asserted through what a script sees rather
// than by inspecting the table, because that is what a script can tell.
func TestApplyRemovesLocal(t *testing.T) {
	const src = `f() { local v=in; }; f`
	run := func(apply bool) (string, int) {
		t.Helper()
		f, err := syntax.Parse(src, ksh.Dialect())
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		sem := ksh.Semantics()
		diag := ksh.Diagnostics()
		r := &interp.Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &diag, Name: "ksh"}
		if apply {
			ksh.Apply(r)
		}
		st, err := r.Run(context.Background(), f)
		if err != nil {
			t.Fatal(err)
		}
		return buf.String(), st
	}
	// Without the dialect's adjustment the substrate provides local.
	if out, st := run(false); st != 0 || out != "" {
		t.Fatalf("the substrate should provide local: %q status %d", out, st)
	}
	out, st := run(true)
	if !strings.Contains(out, "local: not found") {
		t.Errorf("ksh93 has no local: got %q", out)
	}
	if st != 127 {
		t.Errorf("status = %d, want 127", st)
	}
}

// TestNoAmpersandRedirect pins the build this panel measures. `&>` is the
// divergence syntax.Dialect names as the reason its fields are called after
// constructs rather than after shells: ksh93u+ 2012 has no `&>` and
// ksh93u+m does, twelve years apart under the same name.
func TestNoAmpersandRedirect(t *testing.T) {
	if ksh.Dialect().AmpersandRedirect {
		t.Error("ksh93u+ 2012 has no &>, and treating it as one operator silently redirects what should have been backgrounded")
	}
}

// TestUnterminatedNamesTheInnermostUnclosedKeyword is ksh93's view, and the
// one with an irregularity in it: a `do` inside a while is named and a `do`
// inside a `for` is not, which is measured rather than derived.
func TestUnterminatedNamesTheInnermostUnclosedKeyword(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"if", "syntax error at line 1: `if' unmatched"},
		{"if true; then echo x", "syntax error at line 1: `then' unmatched"},
		{"if true; then echo x; else echo y", "syntax error at line 1: `else' unmatched"},
		{"while true; do echo x", "syntax error at line 1: `do' unmatched"},
		{"for i in a; do echo x", "syntax error at line 1: `for' unmatched"},
		// The line ksh93 embeds is where the input ran out, not where the
		// construct began — the two differ only across lines, which is why a
		// single-line case could not tell them apart.
		{"if true; then\necho x", "syntax error at line 2: `then' unmatched"},
	} {
		_, err := syntax.Parse(tc.src, ksh.Dialect())
		if got := ksh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestASourcedFileIsNamedByTheBuiltin is ksh93 alone: a diagnostic about a
// file `.` read names `.` rather than the file, where the other three name
// the path.
func TestASourcedFileIsNamedByTheBuiltin(t *testing.T) {
	d := ksh.Diagnostics()
	if !d.SourceFileIsTheBuiltin {
		t.Error("a sourced file should be named by the builtin that read it")
	}
	if got, want := d.SourceFileNaming, interp.SourceBeforeLocation; got != want {
		t.Errorf("got %v, want it named before the location", got)
	}
}

// TestArithmeticFailuresAreTerse is ksh93's shape.
func TestArithmeticFailuresAreTerse(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"echo $((1 2))", "1 2: arithmetic syntax error"},
		{"echo $((1+))", "1+: more tokens expected"},
	} {
		_, err := syntax.Parse(tc.src, ksh.Dialect())
		if got := ksh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestUnexpectedTokensCarryTheLine is ksh93's shape: the line rides inside the
// message, because ksh93 prints no location of its own for a parse failure.
func TestUnexpectedTokensCarryTheLine(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"echo )", "syntax error at line 1: `)' unexpected"},
		{"echo x\necho )", "syntax error at line 2: `)' unexpected"},
	} {
		_, err := syntax.Parse(tc.src, ksh.Dialect())
		if got := ksh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestKshHasItsOwnNamesForThings covers four answers that are ksh93's alone,
// three of them wordings and one a grammar flag.
func TestKshHasItsOwnNamesForThings(t *testing.T) {
	// `times` is a reserved word, so a word after it does not parse at all.
	if _, err := syntax.Parse("times foo", ksh.Dialect()); err == nil {
		t.Error("`times foo` should not parse where times is reserved")
	} else if got, want := ksh.Diagnostics().ParseFailure(err),
		"syntax error at line 1: `foo' unexpected"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// And `times` on its own still parses, which is the half a flag like this
	// is easiest to break.
	if _, err := syntax.Parse("times", ksh.Dialect()); err != nil {
		t.Errorf("`times` alone should still parse: %v", err)
	}

	// An operator ksh93 cannot read is a syntax error naming the character,
	// not a bad substitution.
	_, err := syntax.Parse("echo ${x^^}", ksh.Dialect())
	if got, want := ksh.Diagnostics().ParseFailure(err),
		"syntax error at line 1: `^' unexpected"; got != want {
		t.Errorf("${x^^}: got %q, want %q", got, want)
	}

	d := ksh.Diagnostics()
	if !d.DotNoOperandUnprefixed {
		t.Error("`.` with no operand should print its usage bare")
	}
	if got, want := d.InvalidNumber, "%[1]s: parameter not set"; got != want {
		t.Errorf("InvalidNumber = %q, want %q — a value that is not a number is a name here", got, want)
	}
}

// TestPrintfAnswers covers the two that are ksh93's alone: `\cX` is a control
// character rather than two characters or a full stop, and its output reaches
// the reader before its complaint about the rest.
func TestPrintfAnswers(t *testing.T) {
	s := ksh.Semantics()
	if got, want := s.PrintfBackslashC, interp.PrintfBackslashCControl; got != want {
		t.Errorf("PrintfBackslashC = %v, want %v", got, want)
	}
	if got, want := s.PrintfOutputPrecedesComplaint, interp.Yes; got != want {
		t.Errorf("PrintfOutputPrecedesComplaint = %v, want %v", got, want)
	}
	if got, want := s.PrintfQuote, interp.PrintfQuoteSingle; got != want {
		t.Errorf("PrintfQuote = %v, want %v", got, want)
	}
}

// TestCdAnswers: ksh93 brackets the reason and has one message for both of
// the variables bash names separately.
func TestCdAnswers(t *testing.T) {
	d := ksh.Diagnostics()
	if got, want := d.CdCannotChange, "cd: %[1]s: [%[2]s]"; got != want {
		t.Errorf("CdCannotChange = %q, want %q", got, want)
	}
	if d.CdHomeNotSet != d.CdOldpwdNotSet || d.CdHomeNotSet != "cd: bad directory" {
		t.Errorf("one message for both, got %q and %q", d.CdHomeNotSet, d.CdOldpwdNotSet)
	}
}

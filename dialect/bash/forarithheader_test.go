// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

// forHeaderDiagnostic renders what bash says about a header it will not parse,
// through the same path the front end uses, and fails a source it accepts.
func forHeaderDiagnostic(t *testing.T, src string) string {
	t.Helper()
	_, err := syntax.Parse(src, bash.Dialect())
	if err == nil {
		t.Fatalf("%s: parsed, want a refusal", src)
	}
	return bash.Diagnostics().ParseDiagnostic("bash", "-c", err, src)
}

// What this shell says about a C-style `for` header whose separator count is
// wrong, and it is two sentences rather than one.
//
// The refusal is the point and the wording is how it is checked. A header with
// fewer than two separators has no condition, and an absent condition is true,
// so accepting one is not a wrong answer but an unbounded one: `for (()); do
// echo x; done` printed for as long as it was left running where bash executes
// none of the script (#2225).
//
// Measured 2026-09-12 on bash 5.3.15, and identically on 3.2.57 and on the
// same binary under an argv[0] of `sh`. Both rendered lines are asserted,
// because the second is the part an implementation is most likely to get
// wrong: it is the *header* rather than the offending source line this dialect
// echoes everywhere else.
func TestAForHeaderWithTooFewSeparatorsIsRefused(t *testing.T) {
	for _, header := range []string{"(())", "(( ))", "((;))", "((1;2))", "((i=0))"} {
		src := "for " + header + "; do echo x; break; done\n"
		want := "bash: -c: line 1: syntax error: arithmetic expression required\n" +
			"bash: -c: line 1: syntax error: `" + header + "'\n"
		if got := forHeaderDiagnostic(t, src); got != want {
			t.Errorf("%q\n got %q\nwant %q", src, got, want)
		}
	}
}

// And a *third* separator is the other sentence, which is why the two are
// separate kinds. An implementation with one message for every bad header
// looks right on the cases above and is wrong on these.
func TestAForHeaderWithTooManySeparatorsIsRefusedDifferently(t *testing.T) {
	for _, header := range []string{"((;;;))", "((;;;;))", "((1;2;3;4))", "(( ; ; ; ))"} {
		src := "for " + header + "; do echo x; break; done\n"
		want := "bash: -c: line 1: syntax error: `;' unexpected\n" +
			"bash: -c: line 1: syntax error: `" + header + "'\n"
		if got := forHeaderDiagnostic(t, src); got != want {
			t.Errorf("%q\n got %q\nwant %q", src, got, want)
		}
	}
}

// The refusal is the parser's, so nothing in the script runs — which is the
// half that separates it from a complaint the loop raises when it is reached,
// and the half that bounds the output. It is blamed on the line the header
// *opens* on, and it carries this dialect's syntax-error status.
func TestARefusedForHeaderStopsTheWholeScript(t *testing.T) {
	const src = "echo before\nfor (()); do echo x; break; done\necho after\n"
	want := "bash: -c: line 2: syntax error: arithmetic expression required\n" +
		"bash: -c: line 2: syntax error: `(())'\n"
	if got := forHeaderDiagnostic(t, src); got != want {
		t.Errorf("\n got %q\nwant %q", got, want)
	}
	if st := bash.Diagnostics().SyntaxErrorStatus; st != 2 {
		t.Errorf("syntax-error status %d, want 2", st)
	}
}

// A header written over several lines is echoed over several lines, and still
// blamed on its first. That is what says the echo is the header and not the
// source line: the source line would be one of four, and the one holding the
// `))` is the only one a reader could mistake for it.
func TestARefusedForHeaderIsEchoedWhole(t *testing.T) {
	const src = "echo before\nfor ((\ni=0\n)); do echo x; break; done\n"
	want := "bash: -c: line 2: syntax error: arithmetic expression required\n" +
		"bash: -c: line 2: syntax error: `((\ni=0\n))'\n"
	if got := forHeaderDiagnostic(t, src); got != want {
		t.Errorf("\n got %q\nwant %q", got, want)
	}
}

// Exactly two separators is still the loop it always was, in every shape the
// spec says may be empty — and the empty header is the endless loop, which a
// check that could not tell "empty section" from "missing separator" would
// have taken with it.
func TestAForHeaderWithTwoSeparatorsStillRuns(t *testing.T) {
	for src, want := range map[string]string{
		"for ((;;)); do echo x; break; done":                 "x\n",
		"for (( ; ; )); do echo x; break; done":              "x\n",
		"for ((i=0;;)); do echo x; break; done":              "x\n",
		"for ((;i<1;)); do echo x; break; done":              "x\n",
		"for ((;;i++)); do echo x; break; done":              "x\n",
		"for ((i=0;i<2;i++)); do printf %s $i; done":         "01",
		"for ((\ni=0;\ni<2;\ni++\n)); do printf %s $i; done": "01",
	} {
		if _, err := syntax.Parse(src+"\n", bash.Dialect()); err != nil {
			t.Fatalf("%q: refused while parsing: %v", src, err)
		}
		out, st := runBash(t, t.TempDir(), src)
		if out != want || st != 0 {
			t.Errorf("%q\n got %q (status %d)\nwant %q at 0", src, out, st, want)
		}
	}
}

// The other `(( … ))` sites are not this one, and the boundary is worth a row
// of its own: an empty expression is a value there rather than a missing
// section, so `while (( ))` and `if (( ))` answer 1 and a bare `(( ))` does
// too. Measured alongside the header table and unchanged by it.
func TestTheOtherArithmeticSitesTakeAnEmptyExpression(t *testing.T) {
	for src, want := range map[string]string{
		"while (( )); do echo x; done; echo st=$?": "st=0\n",
		"if (( )); then echo t; else echo f; fi":   "f\n",
		"(( )); echo st=$?":                        "st=1\n",
	} {
		out, st := runBash(t, t.TempDir(), src)
		if out != want || st != 0 {
			t.Errorf("%q\n got %q (status %d)\nwant %q at 0", src, out, st, want)
		}
	}
	// And a `;` inside one of them is an arithmetic failure when the command
	// runs, not a parse failure — the same text the header refuses outright.
	if _, err := syntax.Parse("(( 1;2 ))\n", bash.Dialect()); err != nil {
		t.Errorf("`(( 1;2 ))` refused while parsing: %v", err)
	}
	out, _ := runBash(t, t.TempDir(), "(( 1;2 )); echo st=$?")
	if !strings.Contains(out, "st=1\n") {
		t.Errorf("got %q, want a run-time refusal and st=1", out)
	}
}

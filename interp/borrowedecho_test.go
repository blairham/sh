// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A dialect that echoes the offending line after a parse failure echoes it
// for text a *builtin* borrowed too — `eval`'s string and a sourced file —
// and not only for what the front end was given.
//
// Only the front end wrote it. So a syntax error in generated text came back
// as a complaint about a token with nothing to attach it to, which is the
// normal case for a program driving the shell: every command arrives as
// `eval <quoted string>`, and the echoed line is the only context the caller
// has (#1728).
func TestBorrowedTextEchoesTheOffendingLine(t *testing.T) {
	for _, c := range []struct{ name, inc, src, want string }{
		{
			"eval's string",
			"",
			"eval 'if; then'\n",
			"sh: eval: line 1: `if; then'\n",
		},
		{
			// Two lines, so a test could not pass by quoting the only line
			// there is: the complaint is about the second.
			"the failing line of it, not the first",
			"",
			"eval 'echo one\nif; then'\n",
			"sh: eval: line 2: `if; then'\n",
		},
		{
			"a sourced file",
			"# a\nif; then\n",
			". ./inc.sh\n",
			"sh: ./inc.sh: line 2: `if; then'\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := borrowedRun(t, c.inc, c.src, true)
			if !strings.Contains(got, c.want) {
				t.Errorf("said %q, want %q in it", got, c.want)
			}
		})
	}
}

// Without the answer there is no second line, which is three of the four
// dialects: they print the complaint and stop.
func TestWithoutThatAnswerBorrowedTextEchoesNothing(t *testing.T) {
	got := borrowedRun(t, "", "eval 'if; then'\n", false)
	if strings.Contains(got, "`if; then'") {
		t.Errorf("said %q, want no echoed line", got)
	}
	if !strings.Contains(got, "eval") {
		t.Errorf("said %q, want the complaint itself", got)
	}
}

// Only a token the grammar did not want gets an echo. Input that simply ran
// out has no offending line to point at, and the shell that writes one writes
// none for it — measured, `eval 'echo "abc'` is one line in bash and not two.
func TestInputThatRanOutIsNotEchoed(t *testing.T) {
	got := borrowedRun(t, "", "eval 'echo \"abc'\n", true)
	if strings.Count(got, "\n") != 1 {
		t.Errorf("said %q, want one line", got)
	}
}

// Both sentences name the same line where `eval`'s text continues the
// caller's — #3194.
//
// The echo is indexed into the borrowed text and printed with the caller's
// number, and those are two numbers rather than one. Passing a single number
// for both made the second sentence contradict the first: bash 5.3 writes
// `eval: line 6:` twice for a two-line `eval` on line 5 whose second line
// will not parse, and this said `line 6` and then `line 2`.
//
// The `eval` is on line 5 and the failure on the text's line 2, so all three
// candidate numbers are different: the text's own is 2, the shifted one is 6,
// and the physical line the string ends on is 6 as well — which is why the
// text's first line has to run and be checked for, since a probe that only
// read the number could not tell the shift from the physical reading.
func TestTheEchoedLineCarriesTheShiftedNumber(t *testing.T) {
	src := ":\n:\n:\n:\neval 'echo one\nif; then'\n"
	got := borrowedRunContinuing(t, src)
	for _, want := range []string{
		"sh: eval: line 6: \";\" unexpected\n",
		"sh: eval: line 6: `if; then'\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("said %q, want %q in it", got, want)
		}
	}
	// And the number the text counts for itself is nowhere in it, which is
	// what the bug wrote.
	if strings.Contains(got, "line 2:") {
		t.Errorf("said %q, want no line 2 in it", got)
	}
}

// borrowedRunContinuing is borrowedRun with the answer that moves `eval`'s
// lines on to the caller's, which is the only place the two numbers differ.
func borrowedRunContinuing(t *testing.T, src string) string {
	t.Helper()
	return borrowedRunWith(t, "", src, true, func(s *Semantics) {
		s.EvalTextContinuesTheCallersLines = Yes
	})
}

// borrowedRun writes inc (when there is one) as inc.sh in a directory of its
// own, runs src there, and returns standard error.
func borrowedRun(t *testing.T, inc, src string, echoes bool) string {
	t.Helper()
	return borrowedRunWith(t, inc, src, echoes, nil)
}

func borrowedRunWith(t *testing.T, inc, src string, echoes bool, tune func(*Semantics)) string {
	t.Helper()
	dir := t.TempDir()
	if inc != "" {
		if err := os.WriteFile(filepath.Join(dir, "inc.sh"), []byte(inc), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	sem := PosixSemantics()
	sem.DotMissingFileFatal = No
	sem.BuiltinSyntaxErrorFatal = No
	if tune != nil {
		tune(&sem)
	}
	dg := Diagnostics{
		Location:               LocationLineWord,
		SourceFileNaming:       SourceBeforeLocation,
		EvalNaming:             SourceBeforeLocation,
		EchoesTheOffendingLine: echoes,
	}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &dg, Name: "sh",
		Stdout: &bytes.Buffer{}, Stderr: &buf, Dir: dir,
		Vars: map[string]string{"PATH": ""},
	})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

// condDiagnostic renders what bash says about a source it will not parse,
// through the same path the front end uses.
func condDiagnostic(t *testing.T, src string) string {
	t.Helper()
	_, err := syntax.Parse(src, bash.Dialect())
	if err == nil {
		t.Fatalf("%s: parsed, want a refusal", src)
	}
	return bash.Diagnostics().ParseDiagnostic("bash", "-c", err, src)
}

// TestAConditionsRefusalIsThreeLines — bash says three things about a token
// refused inside `[[ ]]` and none of them is what it says about a token
// refused anywhere else: a sentence about the construct at the `[[`'s line, a
// shorter `near` at the token's, and the offending line.
//
// Asserted as whole rendered lines rather than by searching, because two of
// the three differ from the ordinary form only in which words they leave out.
func TestAConditionsRefusalIsThreeLines(t *testing.T) {
	out := condDiagnostic(t, "[[ -n x\n -z \"\" ]]")
	want := []string{
		"line 1: syntax error in conditional expression: unexpected token `-z'",
		"line 2: syntax error near `-z'",
		"line 2: ` -z \"\" ]]'",
	}
	got := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%s", len(got), len(want), out)
	}
	for i := range want {
		if !strings.HasSuffix(got[i], want[i]) {
			t.Errorf("line %d = %q, want it to end %q", i+1, got[i], want[i])
		}
	}
	// The two lines carry different numbers on purpose. Written on one line
	// they are the same number, which is the case a copied line still gets
	// right.
	out = condDiagnostic(t, `[[ -n x ; ]]`)
	for _, w := range []string{
		"line 1: syntax error in conditional expression: unexpected token `;'",
		"line 1: syntax error near `;'",
	} {
		if !strings.Contains(out, w) {
			t.Errorf("out %q, want a line ending %q", out, w)
		}
	}
	// And a token refused *outside* a condition keeps the ordinary wording,
	// which is what says the two are different sentences rather than one.
	out = condDiagnostic(t, `echo a (b)`)
	if !strings.Contains(out, "syntax error near unexpected token `('") {
		t.Errorf("out %q, want the ordinary wording outside a condition", out)
	}
	if strings.Contains(out, "conditional expression") {
		t.Errorf("out %q, want no conditional line outside a condition", out)
	}
}

// TestAnUnterminatedConditionIsTwoLines — a `[[` the input ran out inside of
// gets a line of its own naming the closer, and bash writes it for `[[` and
// for nothing else: every other unterminated construct gets the one ordinary
// line. Measured, and the reason it is a second field rather than the one the
// refused-token preamble uses.
func TestAnUnterminatedConditionIsTwoLines(t *testing.T) {
	out := condDiagnostic(t, `[[ -n x`)
	want := []string{
		"line 1: unexpected EOF while looking for `]]'",
		"line 2: syntax error: unexpected end of file from `[[' command on line 1",
	}
	got := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%s", len(got), len(want), out)
	}
	for i := range want {
		if !strings.HasSuffix(got[i], want[i]) {
			t.Errorf("line %d = %q, want it to end %q", i+1, got[i], want[i])
		}
	}
	// Every other construct left open gets one line, which is what says this
	// is `[[`'s and not a rule about running out of input.
	for _, src := range []string{"if true; then", "for i in a; do", "case x in", "{ echo a", "( echo a"} {
		out := condDiagnostic(t, src)
		if n := strings.Count(strings.TrimRight(out, "\n"), "\n") + 1; n != 1 {
			t.Errorf("%s: %d lines, want 1:\n%s", src, n, out)
		}
		if strings.Contains(out, "looking for") {
			t.Errorf("%s: got the condition's extra line:\n%s", src, out)
		}
	}
}

// TestAMissingConditionNamesTheTokenItStoppedOn — the five places the grammar
// wants a condition and does not find one, which until #2909 wrote the
// parser's own prose.
//
// bash writes a different sentence about the construct here from the one
// above, and for the closer itself writes none at all — so these are asserted
// as whole renderings, the line count included. Measured 2026-09-15 on bash
// 5.3.15 over `-c`.
func TestAMissingConditionNamesTheTokenItStoppedOn(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want []string
	}{
		// The closer standing where a condition should have begun: two lines,
		// and no sentence about the construct in front of them.
		{"[[ ]]", []string{"line 1: syntax error near `]]'", "line 1: `[[ ]]'"}},
		{"[[ ! ]]", []string{"line 1: syntax error near `]]'", "line 1: `[[ ! ]]'"}},
		{"[[ a && ]]", []string{"line 1: syntax error near `]]'", "line 1: `[[ a && ]]'"}},
		{"[[ a || ]]", []string{"line 1: syntax error near `]]'", "line 1: `[[ a || ]]'"}},
		// Any other token there is refused inside the condition and named in
		// a sentence of its own.
		{"[[ ; ]]", []string{
			"line 1: unexpected token `;' in conditional command",
			"line 1: syntax error near `;'",
			"line 1: `[[ ; ]]'",
		}},
		{"[[ ) ]]", []string{
			"line 1: unexpected token `)' in conditional command",
			"line 1: syntax error near `)'",
			"line 1: `[[ ) ]]'",
		}},
		// And each `(` the refusal stood inside adds a line of its own,
		// between the two.
		{"[[ ( ]]", []string{
			"line 1: expected `)'",
			"line 1: syntax error near `]]'",
			"line 1: `[[ ( ]]'",
		}},
		{"[[ ( ) ]]", []string{
			"line 1: unexpected token `)' in conditional command",
			"line 1: expected `)'",
			"line 1: syntax error near `)'",
			"line 1: `[[ ( ) ]]'",
		}},
		{"[[ ( ( ) ) ]]", []string{
			"line 1: unexpected token `)' in conditional command",
			"line 1: expected `)'",
			"line 1: expected `)'",
			"line 1: syntax error near `)'",
			"line 1: `[[ ( ( ) ) ]]'",
		}},
		// A group that closed before the failure is not still open, which is
		// what keeps the count a depth rather than a tally of parentheses
		// seen.
		{"[[ a && ( ) ]]", []string{
			"line 1: unexpected token `)' in conditional command",
			"line 1: expected `)'",
			"line 1: syntax error near `)'",
			"line 1: `[[ a && ( ) ]]'",
		}},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out := condDiagnostic(t, tc.src)
			got := strings.Split(strings.TrimRight(out, "\n"), "\n")
			if len(got) != len(tc.want) {
				t.Fatalf("got %d lines, want %d:\n%s", len(got), len(tc.want), out)
			}
			for i := range tc.want {
				if !strings.HasSuffix(got[i], tc.want[i]) {
					t.Errorf("line %d = %q, want it to end %q", i+1, got[i], tc.want[i])
				}
			}
		})
	}

	// None of this reaches a token refused *after* a condition has been read,
	// which keeps its own sentence — the one the test above asserts.
	out := condDiagnostic(t, `[[ -n x ; ]]`)
	if strings.Contains(out, "in conditional command") {
		t.Errorf("out %q, want the other conditional sentence here", out)
	}
	if strings.Contains(out, "expected `)'") {
		t.Errorf("out %q, want no group line where no group was open", out)
	}
}

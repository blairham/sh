// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
)

// forHeaderDiagnostic renders what zsh says about a header it will not parse.
func forHeaderDiagnostic(t *testing.T, src string) string {
	t.Helper()
	_, err := parseZsh(src)
	if err == nil {
		t.Fatalf("%s: parsed, want a refusal", src)
	}
	return zsh.Diagnostics().ParseDiagnostic("zsh", "", err, src)
}

// A C-style `for` header with fewer than two separators is refused here as it
// is everywhere in the panel, and this shell names the text of the header's
// **last** section — the whole of it, blanks trimmed, rather than a token.
//
// Measured 2026-09-12 on zsh 5.9.2 (#2225).
func TestAForHeaderWithTooFewSeparatorsNamesTheLastSection(t *testing.T) {
	for header, near := range map[string]string{
		"((i=0))":   "i=0",
		"((abc))":   "abc",
		"((1;2))":   "2",
		"((;2))":    "2",
		"(( x=1 ))": "x=1",
	} {
		src := "for " + header + "; do echo x; break; done\n"
		want := "zsh:1: parse error near `" + near + "'\n"
		if got := forHeaderDiagnostic(t, src); got != want {
			t.Errorf("%q\n got %q\nwant %q", src, got, want)
		}
	}
}

// And where that section is empty there is nothing to name, so the sentence
// stops. Two wordings for one refusal, which is why the dialect carries two
// fields: a single format would print a sentence ending in an empty pair of quotes here.
func TestAForHeaderWithNothingToNameSaysOnlyParseError(t *testing.T) {
	for _, header := range []string{"(())", "(( ))", "((;))", "((1;))", "(( x ; ))"} {
		src := "for " + header + "; do echo x; break; done\n"
		const want = "zsh:1: parse error\n"
		if got := forHeaderDiagnostic(t, src); got != want {
			t.Errorf("%q\n got %q\nwant %q", src, got, want)
		}
	}
}

// More than two separators is **accepted** here, which is the half of #2225
// that is a dialect's answer rather than a correction: everything past the
// second `;` belongs to the third expression, so the loop parses and a body
// that breaks on its first pass never reaches the leftover text.
func TestAForHeaderWithExtraSeparatorsIsTaken(t *testing.T) {
	for _, src := range []string{
		"for ((;;;)); do echo body; break; done",
		"for ((;;;;)); do echo body; break; done",
		"for ((1;2;3;4)); do echo body; break; done",
		"for (( ; ; ; )); do echo body; break; done",
	} {
		if _, err := parseZsh(src + "\n"); err != nil {
			t.Fatalf("%q: refused while parsing: %v", src, err)
		}
		out, st := runZsh(t, t.TempDir(), src)
		if out != "body\n" || st != 0 {
			t.Errorf("%q\n got %q (status %d)\nwant \"body\\n\" at 0", src, out, st)
		}
	}
}

// And a loop that *does* reach the third expression finds text that is not an
// expression, because the fold left the separator in it. One pass runs and the
// arithmetic then fails, which is what makes the acceptance bounded.
func TestTheFoldedSectionFailsAsArithmetic(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "for ((i=0;i<2;i++;i=9)); do echo b=$i; done")
	const want = "b=0\nzsh:1: bad math expression: illegal character: ;\n"
	if out != want || st != 1 {
		t.Errorf("\n got %q (status %d)\nwant %q at 1", out, st, want)
	}
}

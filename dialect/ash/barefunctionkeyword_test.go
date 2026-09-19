// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/syntax"
)

// This shell has the `function` keyword, and refuses it standing alone by
// naming the token.
//
// The column #3732 left to measure guessed it had no keyword and would answer
// as dash does — running `function` as a command name at 127. It does not.
// Measured 2026-09-19 in the digest-pinned Alpine image internal/oracle
// reaches, BusyBox v1.37.0, script file under `env -i PATH=/usr/bin:/bin
// LC_ALL=C` with standard input on the null device:
//
//	function f { echo B; }; f       B, status 0 — the keyword is here
//	function                        syntax error: unexpected newline (expecting word)
//	function; echo after            syntax error: unexpected ";" (expecting word)
//	function & echo after           syntax error: unexpected "&" (expecting word)
//	function | echo after           syntax error: unexpected "|" (expecting word)
//	function && echo after          syntax error: unexpected "&&" (expecting word)
//	function )                      syntax error: unexpected ")" (expecting word)
//
// Two halves of that wording were deferred here to the `case` refusal, which
// is where they were settled (#3758): the `(expecting word)` tail needed an
// expectation written without quotation marks, which is now
// syntax.Error.ExpectedIsAClass plus Diagnostics.SyntaxExpectingClass, and
// the line a refusal on a newline names was one further on here than this
// parser said, which was Diagnostics.UnexpectedNewlineIsOnTheNextLine going
// unheld in this dialect. Both are asserted below now, so the rows are whole.
func TestTheKeywordWithNoNameNamesTheTokenHere(t *testing.T) {
	if ash.Dialect().BareFunctionKeyword {
		t.Fatal("BareFunctionKeyword is on, so the keyword would stand alone here")
	}
	if !ash.Dialect().FunctionKeyword {
		t.Fatal("FunctionKeyword is off, so this column would have no keyword to refuse")
	}
	for _, tc := range []struct {
		src, token string
		line       int
	}{
		{"function\n", `unexpected newline (expecting word)`, 2},
		{"function; echo after\n", `unexpected ";" (expecting word)`, 1},
		{"function & echo after\n", `unexpected "&" (expecting word)`, 1},
		{"function | echo after\n", `unexpected "|" (expecting word)`, 1},
		{"function && echo after\n", `unexpected "&&" (expecting word)`, 1},
		{"function )\n", `unexpected ")" (expecting word)`, 1},
	} {
		_, err := syntax.Parse(tc.src, ash.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		got := ash.Diagnostics().ParseDiagnostic("<shell>", "", err, tc.src)
		if !strings.Contains(got, tc.token) {
			t.Errorf("%q said %q, want the token named as %q", tc.src, got, tc.token)
		}
		if line := ash.Diagnostics().ParseFailureLine(err); line != tc.line {
			t.Errorf("%q is on line %d, want %d", tc.src, line, tc.line)
		}
	}
	// And the keyword with a name is untouched, which is what says the
	// divergence was the keyword standing alone.
	if _, err := syntax.Parse("function f { echo B; }\n", ash.Dialect()); err != nil {
		t.Errorf("`function f { echo B; }` was refused: %v", err)
	}
}

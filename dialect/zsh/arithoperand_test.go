// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// arithSentence is what this dialect says about an expression it refused. The
// refusal is found while parsing, so there is no run to observe it from.
func arithSentence(t *testing.T, expr string) string {
	t.Helper()
	_, err := syntax.Parse(`echo "$((`+expr+`))"`, zsh.Dialect())
	if err == nil {
		t.Fatalf("$((%s)) parsed, want a failure", expr)
	}
	d := zsh.Diagnostics()
	return d.ParseFailure(err)
}

// The same split, said the other way round: this shell names the end of the
// string when the expression ran out and names the text when it found
// something that could not begin a value. Measured against zsh 5.9.2
// (2026-09-05).
func TestAMissingOperandNamesTheEndOrTheText(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"1+", "bad math expression: operand expected at end of string"},
		{"~", "bad math expression: operand expected at end of string"},
		{"%", "bad math expression: operand expected at `%'"},
		{"1+&2", "bad math expression: operand expected at `&2'"},
		{"1+*", "bad math expression: operand expected at `*'"},
	} {
		if got := arithSentence(t, tc.src); got != tc.want {
			t.Errorf("$((%s)): got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// The text named runs from the refused byte to the end of the expression
// rather than being the one byte, which is what makes it the *text* rather
// than a token.
func TestTheTextNamedRunsToTheEndOfTheExpression(t *testing.T) {
	want := "bad math expression: operand expected at `&2+3'"
	if got := arithSentence(t, "1+&2+3"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// This shell blames a substring's offset alone, so it is also the one that
// reads the offset alone: `${x:1+:2}` still ran out here where the shell that
// names the whole range found a token.
func TestASubstringOffsetIsReadAlone(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `x=abcdef; echo "${x:1+:2}"`)
	want := "bad math expression: operand expected at end of string"
	if !strings.Contains(out, want) {
		t.Errorf("got %q, want it to contain %q", out, want)
	}
}

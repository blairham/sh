// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// arithLoc is what this shell writes in front of a complaint about an
// expression, and arithLine is the whole line it writes.
//
// Observed from a run. It used to be observed from a parse, because the
// expression was read as part of reading the file — which no shell in the
// panel does: the complaint comes when the command runs (#865). The cases
// below are about the sentence, and the location is asserted with it rather
// than trimmed off, so a complaint that moved would be caught here too.
const arithLoc = "zsh:1: "

func arithLine(t *testing.T, expr string) string {
	t.Helper()
	out, _ := answersRun(t, `echo "$((`+expr+`))"`)
	return out
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
		if got := arithLine(t, tc.src); got != arithLoc+tc.want+"\n" {
			t.Errorf("$((%s)): got %q, want %q", tc.src, got, arithLoc+tc.want+"\n")
		}
	}
}

// The text named runs from the refused byte to the end of the expression
// rather than being the one byte, which is what makes it the *text* rather
// than a token.
func TestTheTextNamedRunsToTheEndOfTheExpression(t *testing.T) {
	want := "bad math expression: operand expected at `&2+3'"
	if got := arithLine(t, "1+&2+3"); got != arithLoc+want+"\n" {
		t.Errorf("got %q, want %q", got, arithLoc+want+"\n")
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

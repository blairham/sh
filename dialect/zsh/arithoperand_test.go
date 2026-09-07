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

// This shell has a *third* sentence, for a byte its arithmetic reader refuses
// as part of no token at all — and it gives that sentence only where the
// expression could legally have stopped. Measured against zsh 5.9.2
// (2026-09-07), by trying every ASCII punctuation byte in five positions.
func TestAByteTheReaderRefusesAnswersForItself(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// Nothing read yet.
		{"@", "bad math expression: illegal character: @"},
		{"{", "bad math expression: illegal character: {"},
		{"}", "bad math expression: illegal character: }"},
		{";", "bad math expression: illegal character: ;"},
		{"  @  ", "bad math expression: illegal character: @"},
		{"@1", "bad math expression: illegal character: @"},
		// Where an operator belonged.
		{"1 @", "bad math expression: illegal character: @"},
		{"1@", "bad math expression: illegal character: @"},
		{"a@", "bad math expression: illegal character: @"},
		{"(1) @", "bad math expression: illegal character: @"},
		{"1+2 @", "bad math expression: illegal character: @"},
		{"1 ;", "bad math expression: illegal character: ;"},
		{"1 {", "bad math expression: illegal character: {"},
		{"1 }", "bad math expression: illegal character: }"},
		// The byte alone, not the run and not the rest of the text — which
		// is what separates this sentence from the operand one, since the two
		// name different things about the same failure.
		{"1 @@", "bad math expression: illegal character: @"},
		// And where an operand was wanted, the same byte gets the *other*
		// sentence. This is the half that makes it positional rather than a
		// property of the byte, and it is the row a byte table alone would
		// get wrong.
		{"1+@", "bad math expression: operand expected at `@'"},
		{"+@", "bad math expression: operand expected at `@'"},
		{"~@", "bad math expression: operand expected at `@'"},
		{"1?2:@", "bad math expression: operand expected at `@'"},
		{"1+;", "bad math expression: operand expected at `;'"},
	} {
		if got := arithLine(t, tc.src); got != arithLoc+tc.want+"\n" {
			t.Errorf("$((%s)): got %q, want %q", tc.src, got, arithLoc+tc.want+"\n")
		}
	}
}

// A byte this reader does *not* refuse keeps the sentence it had, in the same
// shell that refuses the others — which is what makes the table a table.
func TestAByteTheReaderAcceptsKeepsItsOwnSentence(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"%", "bad math expression: operand expected at `%'"},
		{"1+", "bad math expression: operand expected at end of string"},
		{"1 2", "bad math expression: operator expected at `2'"},
	} {
		if got := arithLine(t, tc.src); got != arithLoc+tc.want+"\n" {
			t.Errorf("$((%s)): got %q, want %q", tc.src, got, arithLoc+tc.want+"\n")
		}
	}
}

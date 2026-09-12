// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// A newline standing where the grammar wanted something else is three facts
// about one token, and each of them is a dialect's answer.
//
// Measured 2026-09-12 over a script holding `echo a`, `for` and `done`:
//
//	dash     <script>: 3: Syntax error: newline unexpected
//	bash 5.3 <script>: line 2: syntax error near unexpected token `newline'
//	ksh93    <script>: syntax error at line 3: `newline' unexpected
//	zsh      <script>:3: parse error near `\n'
//
// Three of the four blame the line the newline **ends**, dash leaves it
// unquoted where it quotes an operator, and zsh spells it `\n`. The same
// split shows on the `-c` route and for a newline anywhere in a longer
// script (#1364).

// refusedNewline parses a snippet whose refused token is a newline and hands
// back the error, so a row below can ask the sentence and the line of it.
func refusedNewline(t *testing.T, src string) error {
	t.Helper()
	_, err := syntax.Parse(src, syntax.Core())
	if err == nil {
		t.Fatalf("%s: parsed cleanly, want a refusal", src)
	}
	var se *syntax.Error
	if !errors.As(err, &se) {
		t.Fatalf("%s: err = %v, want a *syntax.Error", src, err)
	}
	if se.Class != syntax.ClassNewline {
		t.Fatalf("%s: class = %v, want the newline's own class", src, se.Class)
	}
	return err
}

// The newline is a class of its own, so a dialect can say something about it
// that it does not say about an operator: the two share a sentence today in
// two of the four, which is exactly why the distinction has to be in the
// class and not in the wording.
func TestARefusedNewlineIsItsOwnClass(t *testing.T) {
	const src = "echo a\nfor\ndone"
	quoted := Diagnostics{SyntaxUnexpected: `"%[1]s" unexpected`}
	if got := quoted.ParseFailure(refusedNewline(t, src)); !strings.Contains(got, `"newline" unexpected`) {
		t.Errorf("= %q, want the shared sentence where the dialect says nothing else", got)
	}
	bare := quoted
	bare.SyntaxUnexpectedNewline = "newline unexpected"
	if got := bare.ParseFailure(refusedNewline(t, src)); got != "newline unexpected" {
		t.Errorf("= %q, want the newline's own sentence", got)
	}
	// And it reaches the newline alone: an operator keeps the sentence it
	// had, which is what says this is not a second spelling of
	// SyntaxUnexpected.
	_, err := syntax.Parse("case x in\n;;\nesac", syntax.Core())
	if err == nil {
		t.Fatal("parsed cleanly, want a refusal")
	}
	if a, b := quoted.ParseFailure(err), bare.ParseFailure(err); a != b {
		t.Errorf("operator = %q with the newline wording and %q without it, want no difference", b, a)
	}
}

// The line is the one the newline ends, wherever in the text it stands, and
// it reaches both places a line is written: the location a report is put at,
// and the verb of the one dialect that carries the number inside its sentence.
func TestARefusedNewlineMayBeBlamedOnTheLineItEnds(t *testing.T) {
	for _, tc := range []struct {
		src      string
		at, ends int
	}{
		{"for\ndone", 1, 2},
		{"echo a\nfor\ndone", 2, 3},
		{"echo a\necho b\necho c\nfor\ndone", 4, 5},
	} {
		err := refusedNewline(t, tc.src)
		here := Diagnostics{SyntaxUnexpected: "at line %[3]d: `%[1]s'"}
		next := here
		next.UnexpectedNewlineIsOnTheNextLine = true

		if got := here.ParseFailureLine(err); got != tc.at {
			t.Errorf("%q: line = %d, want %d — where the newline stands", tc.src, got, tc.at)
		}
		if got := next.ParseFailureLine(err); got != tc.ends {
			t.Errorf("%q: line = %d, want %d — where it ends", tc.src, got, tc.ends)
		}
		// The sentence's own verb, which is a second reader of the same
		// number and disagreed with the location while only one asked.
		if got, want := next.ParseFailure(err), "at line "; !strings.HasPrefix(got, want) {
			t.Fatalf("%q: = %q, want a sentence carrying the line", tc.src, got)
		} else if !strings.Contains(got, "at line "+strconv.Itoa(tc.ends)) {
			t.Errorf("%q: sentence = %q, want line %d in it", tc.src, got, tc.ends)
		}
	}
}

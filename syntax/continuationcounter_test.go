// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// The line an unterminated construct is reported at is a **counter** in the
// shell whose wording names it, not a position in the text: it counts how
// many more times the grammar asked for input after the text ran out.
//
// Measured 2026-09-23 on bash 5.3.15 in the pinned image, each text one line
// long inside an `eval` so the count is read straight off the diagnostic. The
// pairs are the whole of the finding — the same construct with and without a
// trailing backslash — because a table of only the backslash column could be
// fitted with a constant and would say nothing about why.
func TestTheEndLineCountsHowOftenTheGrammarAskedAgain(t *testing.T) {
	t.Parallel()
	d := Core()
	d.FunctionKeyword = true
	for _, tc := range []struct {
		src  string
		want int
		why  string
	}{
		// Every construct asks once for its own terminator.
		{`X() { (a)>b`, 2, "a brace wants `}`"},
		{`X() { echo a`, 2, "the same with no redirection"},
		{`if true; then echo a`, 2, "an `if` wants `fi`"},
		{`while true; do echo a`, 2, "a `while` wants `done`"},
		{`( echo a`, 2, "a subshell wants `)`"},
		{`case x in a) echo b`, 2, "a `case` wants `;;`"},
		{`for i in a b; do echo`, 2, "a `for` past its `do` wants `done`"},
		{`for i in a b`, 2, "and a `for` before it wants `do`"},

		// A trailing backslash is a line continuation whose line never came,
		// so the grammar asked once more before it asked for the terminator.
		{"X() { (a)>b\\", 3, "brace, continuation"},
		{"X() { echo a\\", 3, "brace with no redirection, continuation"},
		{"if true; then echo a\\", 3, "if, continuation"},
		{"while true; do echo a\\", 3, "while, continuation"},
		{"( echo a\\", 3, "subshell, continuation"},
		{"case x in a) echo b\\", 3, "case, continuation"},
		{"for i in a b; do echo\\", 3, "for past its `do`, continuation"},

		// And the one construct that wants two things rather than one: the
		// word list terminated, and then `do`. Two asks where the rest have
		// one, which is why this is a rule and not a constant.
		{"for i in a b\\", 4, "for with its word list still open, continuation"},

		// A continuation whose line *did* arrive costs nothing, which is the
		// half that says the count is about asking rather than about
		// backslashes.
		{"X() { (a)>b\\\n", 2, "brace, continuation satisfied by a real line"},
	} {
		_, err := Parse(tc.src, d)
		e, ok := err.(*Error)
		if !ok {
			t.Errorf("%q: %v, want an *Error", tc.src, err)
			continue
		}
		if e.EndLine != tc.want {
			t.Errorf("%q (%s): EndLine %d, want %d", tc.src, tc.why, e.EndLine, tc.want)
		}
	}
}

// TestAnEscapedBackslashIsNotAContinuation: the count is about the input
// running out *on* a continuation, so a backslash that escaped another
// backslash has already been satisfied and costs nothing.
func TestAnEscapedBackslashIsNotAContinuation(t *testing.T) {
	t.Parallel()
	d := Core()
	d.FunctionKeyword = true
	_, err := Parse(`X() { echo a\\`, d)
	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("%v, want an *Error", err)
	}
	// The second backslash is the escaped character, not a continuation
	// asking for a line — so this is the plain "brace wants `}`" count.
	if e.EndLine != 2 {
		t.Errorf("EndLine %d, want 2 — an escaped backslash is not an unsatisfied continuation", e.EndLine)
	}
}

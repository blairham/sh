// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// condDiags is the pair of sentences one dialect gives a `[[ ]]` it could not
// read, beside the wording every other column gives such a refusal — so a
// test can see which of the three answered.
func condDiags() Diagnostics {
	return Diagnostics{
		SyntaxUnexpected:          "near `%[1]s'",
		SyntaxUnexpectedWord:      "near `%[1]s'",
		SyntaxError:               "%[1]s",
		ConditionExpected:         "condition expected: %[1]s",
		ConditionExpectedPrefixed: "parse error: condition expected: %[1]s",
	}
}

func condRefusal(t *testing.T, src string, d Diagnostics) string {
	t.Helper()
	dial := syntax.Core()
	dial.DoubleBracket = true
	_, err := syntax.Parse(src, dial)
	if err == nil {
		t.Fatalf("%s: parsed cleanly, want a refusal", src)
	}
	return d.ParseFailure(err)
}

// A `[[ ]]` whose words are no condition is refused in one dialect by a
// sentence about the **group** rather than about the token the parser stopped
// on — and by one of *two* sentences (#2846).
//
// Every row is a measurement from zsh 5.9.2 on 2026-09-15, over `-c`, a script
// file and standard input alike.
//
// The three-word row is the one that separates the rules: a rule derived from
// the two-word rows alone would name the first word there and be wrong, and
// one derived from the three-word row alone would name the middle word at four
// and be wrong the other way.
func TestAConditionThatWillNotReadNamesOneOfItsWords(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"two words name the first, prefixed", `[[ p q ]]`, "parse error: condition expected: p"},
		{"three name the middle, plainly", `[[ p q r ]]`, "condition expected: q"},
		{"four name the first again", `[[ p q r s ]]`, "condition expected: p"},
		{"and so do five", `[[ p q r s t ]]`, "condition expected: p"},
		// A second word shaped like a single-letter test takes the prefixed
		// sentence and the first word, whatever the arity.
		{"a one-letter test in the middle", `[[ p -n q ]]`, "parse error: condition expected: p"},
		{"at four words too", `[[ p -n q r ]]`, "parse error: condition expected: p"},
		// The shape and not a table of operators: `-Q` is no condition that
		// shell has, and `-` and `--` are not operators at all.
		{"a letter nothing has", `[[ p -Q q ]]`, "parse error: condition expected: p"},
		{"a bare dash", `[[ p - q ]]`, "parse error: condition expected: p"},
		{"two of them", `[[ p -- q ]]`, "parse error: condition expected: p"},
		// Elsewhere in the group it is an ordinary word.
		{"a one-letter test past the second word", `[[ p q -n r s ]]`, "condition expected: p"},
		// A binary operator with nothing after it is the same refusal from
		// the other site, and the group is still two words.
		{"an operator with no right operand", `[[ x == ]]`, "parse error: condition expected: x"},
		{"and the numeric one", `[[ x -eq ]]`, "parse error: condition expected: x"},
		// A long `-word` that is a two-operand operator stands between the
		// operands legitimately, so the group rule applies to it.
		{"a word operator at four words", `[[ p -nt q r ]]`, "condition expected: p"},
		// The word is named as it was written: this happens while reading.
		{"a quoted word keeps its quotes", `[[ p "q" r ]]`, `condition expected: "q"`},
		{"and an expansion is not expanded", `[[ $u q ]]`, "parse error: condition expected: $u"},
		// `!` and the connectives are not part of the group, and a
		// parenthesized group is a group of its own.
		{"a negation is not a word of the group", `[[ ! p q r ]]`, "condition expected: q"},
		{"nor is a connective", `[[ p && q r s t ]]`, "condition expected: q"},
		{"the left of one is its own group", `[[ p q r || s ]]`, "condition expected: q"},
		{"and so are parentheses", `[[ ( p q r ) ]]`, "condition expected: q"},
		{"two words inside them", `[[ ( p q ) ]]`, "parse error: condition expected: p"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := condRefusal(t, tc.src, condDiags()); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// And the dialects without the sentences name the token, exactly as they did:
// an empty ConditionExpected is what says a column has no such wording.
func TestAConditionRefusalWithoutTheSentencesNamesTheToken(t *testing.T) {
	d := Diagnostics{SyntaxUnexpected: "near `%[1]s'", SyntaxUnexpectedWord: "near `%[1]s'"}
	for _, tc := range []struct{ src, want string }{
		{`[[ p q ]]`, "near `q'"},
		{`[[ p q r ]]`, "near `q'"},
	} {
		if got := condRefusal(t, tc.src, d); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// A `-word` long enough to be a *named* condition is a different refusal, and
// is deliberately left to the token wording: that shell calls it an unknown
// condition and does so when the condition **runs** — `[[ -zz x ]]` prints the
// line before it first — which is neither of the two sentences here.
func TestANamedConditionWordIsNotThisRefusal(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`[[ p -zz q ]]`, "near `-zz'"},
		{`[[ -zz x ]]`, "near `x'"},
		{`[[ p -prefix q ]]`, "near `-prefix'"},
	} {
		if got := condRefusal(t, tc.src, condDiags()); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// A dialect with one sentence for both states it once: an empty prefixed
// wording falls back to the plain one rather than to nothing.
func TestOneConditionSentenceServesBoth(t *testing.T) {
	d := condDiags()
	d.ConditionExpectedPrefixed = ""
	if got, want := condRefusal(t, `[[ p q ]]`, d), "condition expected: p"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

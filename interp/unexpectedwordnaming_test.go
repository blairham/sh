// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// A word the grammar cannot take is echoed back three different ways, and the
// difference is the *characters*: what the word comes to, or what was written.
//
// Measured 2026-09-12 over a script holding `if true; then echo t; fi W`,
// where the `fi` has already closed the `if`:
//
//	W          bash 5.3 / bash32 / bash-as-sh / zsh   ksh93
//	"zzz"      `"zzz"`                                `zzz`
//	'a b'      `'a b'`                                `a b`
//	a""b       `a""b`                                 `ab`
//	\zzz       `\zzz`                                 `zzz`
//	$x         `$x`                                   `$x`
//	"$x"       `"$x"`                                 `"$x"`
//	"a"~       `"a"~`                                 `a~`
//
// dash names no word at all here — `word unexpected` — so the axis has three
// values and not four (#1239).

func refusedWord(t *testing.T, word string, d Diagnostics) string {
	t.Helper()
	src := "if true; then echo t; fi " + word
	_, err := syntax.Parse(src, syntax.Core())
	if err == nil {
		t.Fatalf("%s: parsed cleanly, want a refusal", src)
	}
	return d.ParseFailure(err)
}

// naming is the same sentence at each of the axis's three values, so a row
// below reads as the three answers to one question.
func naming(v UnexpectedWordNaming) Diagnostics {
	return Diagnostics{SyntaxUnexpected: "near `%[1]s'", UnexpectedWordNaming: v}
}

func TestARefusedWordIsNamedThreeWays(t *testing.T) {
	for _, tc := range []struct {
		word                         string
		comesTo, source, whenExpands string
	}{
		{`"zzz"`, "zzz", `"zzz"`, "zzz"},
		{`'a b'`, "a b", `'a b'`, "a b"},
		{`a""b`, "ab", `a""b`, "ab"},
		{`\zzz`, "zzz", `\zzz`, "zzz"},
		{`$x`, "x", `$x`, `$x`},
		{`"$x"`, "x", `"$x"`, `"$x"`},
		{`"a"~`, "a~", `"a"~`, "a~"},
	} {
		t.Run(tc.word, func(t *testing.T) {
			for _, want := range []struct {
				value UnexpectedWordNaming
				text  string
			}{
				{UnexpectedWordIsWhatItComesTo, tc.comesTo},
				{UnexpectedWordIsSourceText, tc.source},
				{UnexpectedWordIsSourceTextWhenItExpands, tc.whenExpands},
			} {
				got := refusedWord(t, tc.word, naming(want.value))
				if q := "near `" + want.text + "'"; !strings.Contains(got, q) {
					t.Errorf("value %d: = %q, want %q", want.value, got, q)
				}
			}
		})
	}
}

// The last two rows are the whole of the third value: one expansion anywhere
// in the word keeps every quote in it, so "quotes off" and "quotes off unless
// it expands" are not the same answer with a longer name.
func TestOneExpansionKeepsTheWholeWordAsWritten(t *testing.T) {
	d := naming(UnexpectedWordIsSourceTextWhenItExpands)
	withExpansion := refusedWord(t, `"a"$x`, d)
	if !strings.Contains(withExpansion, "near `\"a\"$x'") {
		t.Errorf("= %q, want the quotes kept", withExpansion)
	}
	without := refusedWord(t, `"a"~`, d)
	if !strings.Contains(without, "near `a~'") {
		t.Errorf("= %q, want the quotes off", without)
	}
}

// And the axis reaches a *word* alone: an operator was written as the same
// characters it is named by, so no value of it can move one.
func TestNamingAWordLeavesAnOperatorAlone(t *testing.T) {
	src := "case x in\n;;\nesac"
	_, err := syntax.Parse(src, syntax.Core())
	if err == nil {
		t.Fatal("parsed cleanly, want a refusal")
	}
	first := naming(UnexpectedWordIsWhatItComesTo).ParseFailure(err)
	for _, v := range []UnexpectedWordNaming{
		UnexpectedWordIsSourceText,
		UnexpectedWordIsSourceTextWhenItExpands,
	} {
		if got := naming(v).ParseFailure(err); got != first {
			t.Errorf("value %d: = %q, want %q — an operator has one spelling", v, got, first)
		}
	}
}

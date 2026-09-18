// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// Semantics.DollarSingleOctalPastAByteDropsTheLastDigit, both answers.
//
// A three-digit octal escape whose value is past 255 has two readings: the low
// byte of the whole, and the *first two digits'* byte with the third read and
// thrown away. The third digit is consumed either way, so the text after the
// escape starts in the same place under both — which is what makes this a
// conflict about a value rather than about how far the escape runs.

// octalSem answers this axis and the ones a `$'…'` snippet meets on the way to
// it, with the NUL kept as a byte so a row about the overflow is not really a
// row about what a zero does to the span.
func octalSem(drops Answer) Semantics {
	s := dollarSingleSem(DollarSingleControlAbsent, DollarSingleUnknownKeepsBackslash, DollarSingleNulIsAByte)
	s.DollarSingleOctalPastAByteDropsTheLastDigit = drops
	return s
}

func TestAnOctalEscapePastAByte(t *testing.T) {
	for _, tc := range []struct{ name, src, keeps, drops string }{
		// The controls: a value that fits is the same byte under both, and so
		// is a run whose fourth digit was never part of the escape. A row that
		// differed here would be about the scan and not about the overflow.
		{"a value that fits", `printf '%s' $'\377'`, "\xff", "\xff"},
		{"a two-digit escape", `printf '%s' $'\101'`, "A", "A"},
		{"a fourth digit is text", `printf '%s' $'\1234'`, "S4", "S4"},
		// And the rows the two readings part on.
		{"the first value past a byte", `printf '%s' $'\401'`, "\x01", " "},
		{"one further in", `printf '%s' $'\477'`, "\x3f", "\x27"},
		{"the high half", `printf '%s' $'\600'`, "\x80", "\x30"},
		{"the last three-digit value", `printf '%s' $'\777'`, "\xff", "\x3f"},
		// The third digit is consumed under both, so a fourth digit is text
		// either way and the reading cannot be mistaken for a shorter scan.
		// A zero byte on the left of this one, since 0o400 is 256: what the
		// NUL then does to the text is DollarSingleNul's answer and not this
		// axis's, and the row is kept for the digit after it.
		{"a digit after the escape", `printf '%s' $'\4001'`, "\x001", " 1"},
		{"text on both sides", `printf '%s' $'a\401b'`, "a\x01b", "a b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, withSem(octalSem(No)))
			if out != tc.keeps || st != 0 {
				t.Errorf("keeping the low byte: = %q status %d, want %q", out, st, tc.keeps)
			}
			out, st = run(t, tc.src, withSem(octalSem(Yes)))
			if out != tc.drops || st != 0 {
				t.Errorf("dropping the last digit: = %q status %d, want %q", out, st, tc.drops)
			}
		})
	}
}

// An escape that fits in a byte puts no question to the dialect.
//
// The axis is consulted only where the two readings can differ, so a vector
// with no answer for it still writes `$'\101'` — which is what keeps a
// dialect that never reaches the question from having to answer it.
func TestAnOctalEscapeThatFitsNeverAsksTheAxis(t *testing.T) {
	out, st := run(t, `printf '%s' $'a\101\0z'`, withSem(octalSem(Unspecified)))
	if out != "aA\x00z" || st != 0 {
		t.Errorf("= %q status %d, want %q and 0", out, st, "aA\x00z")
	}
}

// And one that does not fit is refused rather than guessed.
func TestAnUnansweredOctalOverflowIsRefused(t *testing.T) {
	out, st := run(t, `printf '%s' $'\401'`, withSem(octalSem(Unspecified)))
	if st == 0 {
		t.Errorf("= %q status %d, want a failing status", out, st)
	}
}

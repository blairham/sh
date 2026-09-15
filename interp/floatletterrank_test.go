// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// NumericTypeLetterPrecedence — #2419. Named for the axis and never for a
// shell.

// rankedLetters is floatRun with both float letters spelled, each able to
// carry its own number, and the precedence answered.
func rankedLetters(t *testing.T, src string, p NumericTypeLetterPrecedencePolicy) (string, string, int) {
	t.Helper()
	return floatRun(t, src, func(s *Semantics) {
		withFloatLetter(s)
		s.DeclareOptions = "aAEiFprx"
		s.DeclareOptionsTakingANumber = "EF"
		s.FloatFormatLetterE = FloatFormatSignificantDigits
		s.NumericTypeLetterPrecedence = p
		s.DeclareListing = DeclareListingExportSpelled
	}, Diagnostics{})
}

// Which letter the name ends up with when a declaration writes two of the
// three.
//
// Every number here is written *attached* and every pair spans two words, so
// no row depends on where a detached number may stand — that is
// DeclareNumberDetachedOnlyAtTheWordEnd's question, and a table that mixed
// the two could not say which one it was reading.
func TestWhichNumericLetterWinsIsAnAxis(t *testing.T) {
	for _, c := range []struct{ name, src, first, rank string }{
		{
			"the integer letter in front of a float one",
			`typeset -i -F3 v=1.5; typeset -p v`,
			`typeset -i v="1"`, `typeset -F v="1.500"`,
		},
		{
			// The row that says the rank is a *rank* and not "a float
			// letter beats an integer one": the exponent letter wins from
			// behind, where the order reading gives it to the one in front.
			"the plain float letter in front of the exponent one",
			`typeset -F3 -E v=1.5; typeset -p v`,
			`typeset -F v="1.500"`, `typeset -E v="1.5"`,
		},
		{
			// The two controls the readings agree on. Without them a suite
			// could pass by having stopped reading the second letter at all.
			"a float letter in front of the integer one",
			`typeset -F3 -i v=1.5; typeset -p v`,
			`typeset -F v="1.500"`, `typeset -F v="1.500"`,
		},
		{
			"the exponent letter in front of the plain one",
			`typeset -E3 -F v=1.5; typeset -p v`,
			`typeset -E v="1.5"`, `typeset -E v="1.5"`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, st := rankedLetters(t, c.src, NumericLetterFirstWrittenWins)
			if got := strings.TrimSuffix(out, "\n"); got != c.first || st != 0 {
				t.Errorf("first-written: got %q (stderr %q, status %d), want %q", got, errs, st, c.first)
			}
			out, errs, st = rankedLetters(t, c.src, NumericLetterFloatOutranksTheInteger)
			if got := strings.TrimSuffix(out, "\n"); got != c.rank || st != 0 {
				t.Errorf("ranked: got %q (stderr %q, status %d), want %q", got, errs, st, c.rank)
			}
		})
	}
}

// A single numeric letter meets no question at all — under either answer and
// under none, since the axis is asked only where more than one was written.
func TestOneNumericLetterIsNeverRanked(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`typeset -F3 v=1.5; typeset -p v`, `typeset -F v="1.500"`},
		{`typeset -E3 v=1.5; typeset -p v`, `typeset -E v="1.5"`},
	} {
		for _, p := range []NumericTypeLetterPrecedencePolicy{
			NumericLetterFirstWrittenWins,
			NumericLetterFloatOutranksTheInteger,
			NumericLetterPrecedenceUnspecified,
		} {
			out, errs, st := rankedLetters(t, c.src, p)
			if got := strings.TrimSuffix(out, "\n"); got != c.want || st != 0 || errs != "" {
				t.Errorf("%s answered %v: got %q (stderr %q, status %d), want %q",
					c.src, p, got, errs, st, c.want)
			}
		}
	}
}

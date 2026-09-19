// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The base, overflow, empty-expression and consumed-prefix questions, each
// named by its axis and never by a shell.

func arithRun(t *testing.T, src string, set func(*Semantics), dg Diagnostics) (string, int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		sem := CoreSemantics()
		if set != nil {
			set(&sem)
		}
		r.Semantics, r.Diagnostics = &sem, &dg
	})
}

func TestBasesRunToSixtyFourWhereAdmitted(t *testing.T) {
	out, st := arithRun(t, `echo $((36#z)) $((64#z)) $((64#Z)) $((64#_))`, func(s *Semantics) {
		s.ArithBaseAbove36 = Yes
	}, Diagnostics{})
	if st != 0 || strings.TrimSpace(out) != "35 35 61 63" {
		t.Errorf("out=%q st=%d, want the full alphabet read", out, st)
	}
	out, st = arithRun(t, `echo $((37#1))`, func(s *Semantics) {
		s.ArithBaseAbove36 = No
	}, Diagnostics{ArithInvalidBase: "invalid base (must be 2 to 36 inclusive): %[1]s"})
	if st == 0 || !strings.Contains(out, "invalid base (must be 2 to 36 inclusive): 37") {
		t.Errorf("out=%q st=%d, want the capped dialect's refusal", out, st)
	}
}

// TestArithmeticCarriedInADoubleWhereAsked replaces a test that asked only
// `big + 1`, which is the maximum under either reading and so could not tell a
// clamp from a value carried in a double. Each row below fails under the other
// answer.
func TestArithmeticCarriedInADoubleWhereAsked(t *testing.T) {
	carried := func(s *Semantics) { s.ArithValuesAreCarriedInAFloat = Yes }
	word := func(s *Semantics) { s.ArithValuesAreCarriedInAFloat = No }
	for _, tc := range []struct {
		name    string
		snippet string
		set     func(*Semantics)
		want    string
	}{
		// The row the old test had. It stays because it still has to hold,
		// and it is kept company by ones that discriminate.
		{"the maximum plus one", `echo $((9223372036854775807 + 1))`, carried, "9223372036854775807"},
		{"below the minimum", `echo $((-9223372036854775807 - 2))`, carried, "-9223372036854775808"},
		{"the maximum plus one in the word", `echo $((9223372036854775807 + 1))`, word, "-9223372036854775808"},

		// Past the word the two readings part: a clamp says the maximum a
		// second time where the double says what the double holds.
		{"twice the maximum", `echo $((9223372036854775807 * 2))`, carried, "1.84467440737096e+19"},
		{"twice the maximum in the word", `echo $((9223372036854775807 * 2))`, word, "-2"},
		{"two to the sixty-fourth", `echo $((2**64))`, carried, "1.84467440737096e+19"},

		// And under the word it is not an overflow question at all, which is
		// the half a saturation reading cannot express.
		{"one past what a double holds", `echo $((9007199254740993))`, carried, "9007199254740992"},
		{"one past what a double holds in the word", `echo $((9007199254740993))`, word, "9007199254740993"},
		{"a product that rounds", `echo $((3037000499*3037000499))`, carried, "9223372030926248960"},
		{"a quotient that rounds", `echo $((1152921504606846976/3))`, carried, "384307168202282304"},
		{"a bit the double cannot keep", `echo $(((1<<62) | 1))`, carried, "4611686018427387904"},

		// A wide value is an integer still, so the integer operators do
		// integer work on the saturated cast rather than refusing it.
		{"a wide value divided", `echo $((2**64 / 3))`, carried, "3074457345618258432"},
		{"a wide value remaindered", `echo $((2**64 % 3))`, carried, "1"},

		// A numeral the word cannot hold is asked in dialect/ksh instead: the
		// reader takes the dialect's own word for whether the shell has
		// floats at all, and this harness is a shell with no dialect.
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := arithRun(t, tc.snippet, tc.set, Diagnostics{ArithFloatDigits: 15})
			if strings.TrimSpace(out) != tc.want {
				t.Errorf("out=%q, want %q", strings.TrimSpace(out), tc.want)
			}
		})
	}
}

func TestAnEmptyExpressionIsAnAxis(t *testing.T) {
	out, st := arithRun(t, `echo $(( ))`, func(s *Semantics) {
		s.EmptyArithExpressionIsAnError = No
	}, Diagnostics{})
	if st != 0 || strings.TrimSpace(out) != "0" {
		t.Errorf("out=%q st=%d, want zero", out, st)
	}
	out, st = arithRun(t, `echo $(( )); echo after`, func(s *Semantics) {
		s.EmptyArithExpressionIsAnError = Yes
	}, Diagnostics{})
	if st == 0 || !strings.Contains(out, "expecting primary") || strings.Contains(out, "after") {
		t.Errorf("out=%q st=%d, want the primary wanted and the script stopped", out, st)
	}
}

func TestTheErrorNamesTheConsumedPrefix(t *testing.T) {
	dg := Diagnostics{
		ArithError:               `%[1]s: %[2]s (error token is "%[3]s")`,
		DigitTooGreatForBase:     "value too great for base",
		ArithErrorNamesThePrefix: true,
	}
	set := func(s *Semantics) { s.ArithLeadingZeroIsOctal = Yes; s.ArithInvalidOctalDigitIsError = Yes }
	out, _ := arithRun(t, `echo $((08+1))`, set, dg)
	if !strings.Contains(out, `08: value too great for base (error token is "08")`) ||
		strings.Contains(out, "08+1:") {
		t.Errorf("out=%q, want the consumed prefix alone", out)
	}
	out, _ = arithRun(t, `echo $((1+08))`, set, dg)
	if !strings.Contains(out, `1+08: value too great for base (error token is "08")`) {
		t.Errorf("out=%q, want everything consumed named", out)
	}
}

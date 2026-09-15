// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// ArithFloatOverflowIsZero — #2766.
//
// A numeral a double cannot hold is a value the reader has and one of the two
// float columns throws away. This shell threw away *both* the value and the
// reader's complaint and refused the numeral, which is neither reading.
//
// runOverflow names the flags rather than a shell, as this package's rule
// requires: floats on, the formatting one column's, and the axis moved.
func runOverflow(t *testing.T, src string, keepPoint bool, zero Answer) (string, int) {
	t.Helper()
	s := testSemantics()
	s.ArithFloatOverflowIsZero = zero
	dg := Diagnostics{
		ArithFloatDigits:     15,
		ArithFloatKeepsPoint: keepPoint,
		ArithInfinity:        "Inf",
		ArithNotANumber:      "NaN",
	}
	out, st := runGrammar(t, src,
		func(d *syntax.Dialect) { d.ArithFloat = true },
		func(r *Runner) { r.Semantics = &s; r.Diagnostics = &dg })
	return strings.TrimSpace(out), st
}

func TestAFloatNumeralTooLargeForADouble(t *testing.T) {
	t.Run("the saturating reading keeps the infinity the reader produced", func(t *testing.T) {
		// The value and the out-of-range report arrive together, and taking
		// the report alone is what refused the numeral.
		for _, tc := range []struct{ src, want string }{
			{`echo $((1e400))`, "Inf"},
			{`echo $((-1e400))`, "-Inf"},
			{`echo $((1e400+1))`, "Inf"},
			{`x=1e400; echo $((x))`, "Inf"},
		} {
			if got, st := runOverflow(t, tc.src, false, No); got != tc.want || st != 0 {
				t.Errorf("%s = %q status %d, want %q and 0", tc.src, got, st, tc.want)
			}
		}
	})
	t.Run("the losing reading answers a zero, and the numeral's is negative", func(t *testing.T) {
		// The sign is the measurement and not decoration: the unary minus is
		// applied to a zero that already carries one, so the two rows come
		// out the opposite way round from the numerals that produced them.
		for _, tc := range []struct{ src, want string }{
			{`echo $((1e400))`, "-0"},
			{`echo $((-1e400))`, "0"},
			{`echo $((1e400+1))`, "1"},
		} {
			if got, st := runOverflow(t, tc.src, false, Yes); got != tc.want || st != 0 {
				t.Errorf("%s = %q status %d, want %q and 0", tc.src, got, st, tc.want)
			}
		}
	})
	t.Run("a numeral out of a variable is the other reader, and its zero is positive", func(t *testing.T) {
		// The two readers part here the way they part over a leading zero,
		// and in the same column.
		for _, tc := range []struct{ src, want string }{
			{`x=1e400; echo $((x))`, "0"},
			{`x=1e400; echo $((x+1))`, "1"},
			{`x=-1e400; echo $((x))`, "0"},
		} {
			if got, st := runOverflow(t, tc.src, false, Yes); got != tc.want || st != 0 {
				t.Errorf("%s = %q status %d, want %q and 0", tc.src, got, st, tc.want)
			}
		}
	})
	t.Run("a value that overflowed while being computed is not the numeral", func(t *testing.T) {
		// The control the axis's own column supplies: the same magnitude
		// reached by multiplying is an infinity under both readings, so a
		// fix that zeroed every overflow would contradict the measurement it
		// came from.
		for _, a := range []Answer{Yes, No} {
			if got, st := runOverflow(t, `echo $((1e300*1e300))`, false, a); got != "Inf" || st != 0 {
				t.Errorf("answered %v: = %q status %d, want \"Inf\" and 0", a, got, st)
			}
		}
	})
	t.Run("an underflow is zero without the axis being asked", func(t *testing.T) {
		// Both columns answer zero here, so there is nothing to choose — and
		// an unanswered axis must not refuse a line the panel agrees about.
		if got, st := runOverflow(t, `echo $((1e-400))`, true, Unspecified); got != "0." || st != 0 {
			t.Errorf("= %q status %d, want \"0.\" and 0", got, st)
		}
	})
	t.Run("a numeral that is no number at all is still refused", func(t *testing.T) {
		// The out-of-range report is the only one taken as a value. A
		// malformed numeral has no value to take.
		out, st := runOverflow(t, `echo $((1.2.3))`, false, Yes)
		if st == 0 || strings.Contains(out, "0") && !strings.Contains(out, "1.2.3") {
			t.Errorf("= %q status %d, want the numeral refused", out, st)
		}
	})
	t.Run("an unanswered axis is refused rather than guessed", func(t *testing.T) {
		out, st := runOverflow(t, `echo $((1e400))`, false, Unspecified)
		if st == 0 || !strings.Contains(out, "disagree") {
			t.Errorf("= %q status %d, want the axis refused", out, st)
		}
	})
	t.Run("a dialect without floats never reaches the axis", func(t *testing.T) {
		// `1e400` is not a number out of range where there are no floats: it
		// is a word the arithmetic cannot read, refused one step earlier, so
		// an unanswered field must not turn into a second complaint.
		s := testSemantics()
		s.ArithFloatOverflowIsZero = Unspecified
		out, _ := run(t, `echo $((1e400))`, withSem(s))
		if strings.Contains(out, "disagree") {
			t.Errorf("= %q, want no axis refusal where the dialect has no floats", out)
		}
	})
}

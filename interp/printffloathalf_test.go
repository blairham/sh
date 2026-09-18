// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// A floating conversion whose operand falls exactly halfway at the precision
// asked for (#2903).
//
// The axis is moved in both directions and left unanswered on the same
// snippets, because a rounding that is only ever one way cannot be told from
// one nobody consults: the rows that decline are what say it is read.
//
// Two of the rows below are the away reading *not* firing, and they are the
// reason this is an enumeration rather than a bool. A half under a tenth and
// a half the precision reaches no digit of go the other way in the column
// that rounds the rest of them up, so a field that could say only "away"
// would be wrong about both.
func TestAFloatingConversionsExactHalf(t *testing.T) {
	for _, tc := range []struct {
		name   string
		policy PrintfFloatHalfPolicy
		src    string
		want   string
	}{
		{
			name: "to even", policy: PrintfFloatHalfToEven,
			src: `printf '%.0f %.0f %.0f' 2.5 4.5 -2.5`, want: "2 4 -2",
		},
		{
			name: "away above a tenth", policy: PrintfFloatHalfAwayFromZeroAboveATenth,
			src: `printf '%.0f %.0f %.0f' 2.5 4.5 -2.5`, want: "3 5 -3",
		},
		{
			name: "a fraction, to even", policy: PrintfFloatHalfToEven,
			src: `printf '%.1f %.2f' 0.25 0.125`, want: "0.2 0.12",
		},
		{
			name: "a fraction, away", policy: PrintfFloatHalfAwayFromZeroAboveATenth,
			src: `printf '%.1f %.2f' 0.25 0.125`, want: "0.3 0.13",
		},
		{
			// The scientific and shortest styles round at a place of their
			// own, and the same answer reaches both.
			name: "e and g, to even", policy: PrintfFloatHalfToEven,
			src: `printf '%.0e %.2g' 2.5 0.125`, want: "2e+00 0.12",
		},
		{
			name: "e and g, away", policy: PrintfFloatHalfAwayFromZeroAboveATenth,
			src: `printf '%.0e %.2g' 2.5 0.125`, want: "3e+00 0.13",
		},
		{
			// Under a tenth, where the away reading stops: the same answer
			// takes the smaller magnitude and the other takes the even
			// neighbor, so the two part in the opposite direction.
			name: "under a tenth, to even", policy: PrintfFloatHalfToEven,
			src: `printf '%.4f' 0.09375`, want: "0.0938",
		},
		{
			name: "under a tenth, away", policy: PrintfFloatHalfAwayFromZeroAboveATenth,
			src: `printf '%.4f' 0.09375`, want: "0.0937",
		},
		{
			// And a half the precision reaches no digit of, which both
			// answers write the same way. It is here because an "away"
			// reading with no such guard would write `1`.
			name: "no digit kept, to even", policy: PrintfFloatHalfToEven,
			src: `printf '%.0f %.0f' 0.5 -0.5`, want: "0 -0",
		},
		{
			name: "no digit kept, away", policy: PrintfFloatHalfAwayFromZeroAboveATenth,
			src: `printf '%.0f %.0f' 0.5 -0.5`, want: "0 -0",
		},
		{
			// An operand that is not a half at the precision asked for
			// never reaches the axis, which is why the two answers agree
			// here whatever they hold: 1.005 is under the half once it is a
			// double, and 0.3 is nowhere near one.
			name: "not a half, to even", policy: PrintfFloatHalfToEven,
			src: `printf '%.2f %.0f' 1.005 0.3`, want: "1.00 0",
		},
		{
			name: "not a half, away", policy: PrintfFloatHalfAwayFromZeroAboveATenth,
			src: `printf '%.2f %.0f' 1.005 0.3`, want: "1.00 0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfFloatHalf = tc.policy
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// An unanswered axis refuses by name, and only for an operand that really is
// a half: a conversion of anything else is written without a question being
// put to anybody.
func TestAnUnansweredFloatHalfRefusesOnlyAHalf(t *testing.T) {
	sem := printfSem()
	sem.PrintfFloatHalf = PrintfFloatHalfUnspecified

	out, st := run(t, `printf '%.0f' 2.5`, func(r *Runner) { r.Semantics = &sem })
	const want = "sh: printf: a floating conversion's exact half: the shells disagree here and no dialect was chosen\n2"
	if out != want || st != 2 {
		t.Errorf("a half: got %q status %d, want %q and 2", out, st, want)
	}

	out, st = run(t, `printf '%.2f' 1.005`, func(r *Runner) { r.Semantics = &sem })
	if out != "1.00" || st != 0 {
		t.Errorf("not a half: got %q status %d, want %q and 0", out, st, "1.00")
	}
}

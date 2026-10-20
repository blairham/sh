// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// This shell runs the operand of `&&` or `||` that its own short circuit has
// already decided, so an assignment written there takes effect.
//
// The sole holdout in the panel of seven, and the first thing the ash column
// of `make suite` reported on the day it could run at all (#2605). Measured
// against BusyBox v1.37.0 on 2026-09-13, through the container route the
// oracle reaches this shell by; the corpus rows are
// `arith/short-circuit-is-observable` and the five beside it.
//
// It is asserted here and not in `interp` because it is what *this shell*
// answers: the substrate's tests name the axis and run it at both values.
func TestADecidedOperandIsRunAnyway(t *testing.T) {
	for _, tc := range []struct{ name, src, want, why string }{
		{
			"an assignment in the right operand of a false `&&`",
			`x=0; : $((0 && (x = 9))); echo "x=$x"`,
			"x=9",
			"the case the divergence was found on",
		},
		{
			"an assignment in the right operand of a true `||`",
			`y=0; : $((1 || (y = 8))); echo "y=$y"`,
			"y=8",
			"both logical operators, so the axis is not named for one of them by accident",
		},
		{
			"an increment rather than an assignment",
			`x=0; : $((0 && (x++))); echo "x=$x"`,
			"x=1",
			"the other side effect arithmetic has, which says the operand is evaluated rather than that assignment is special",
		},
		{
			"an assignment two levels inside the operand",
			`x=0; y=0; : $((0 && (1 && (x = 1)) && (y = 2))); echo "x=$x y=$y"`,
			"x=1 y=2",
			"nesting is not a bound on it",
		},
		{
			"the value the operator yields is still the operator's",
			`x=0; echo "v=$((0 && (x = 9))) w=$((7 || (x = 9)))"`,
			"v=0 w=1",
			"`0 && anything` is 0 and `7 || anything` is 1 in every shell, which is why the value could never have found this",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src)
			if strings.TrimSpace(out) != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0 — %s", tc.src, out, st, tc.want, tc.why)
			}
		})
	}
}

// And the conditional, which does not move with them.
//
// Measured on the same day and in the same shell: the arm a conditional did
// not take is not evaluated, and an `&&` written inside that arm never gets
// the chance to run its own decided operand. That is what makes the axis a
// property of the two logical operators rather than of a shell that evaluates
// everything it parses — and a fix written as "this dialect evaluates both
// sides" would move all three of these.
func TestAConditionalStillDropsTheArmItDidNotTake(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`w=5; echo "v=$((0 ? (w = 1) : 2)) w=$w"`, "v=2 w=5"},
		{`w=5; echo "v=$((1 ? 2 : (w = 1))) w=$w"`, "v=2 w=5"},
		{`x=0; echo "v=$((0 ? (0 && (x = 9)) : 4)) x=$x"`, "v=4 x=0"},
	} {
		out, st := run(t, tc.src)
		if strings.TrimSpace(out) != tc.want || st != 0 {
			t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// The operand that failed rather than assigned, which is what says the shell
// evaluates it rather than walking it for stores.
//
// `$((0 && (1/0)))` is `divide by zero` at status 2 in BusyBox, where the
// other five columns answer 0 quietly. An implementation that performed the
// assignments it found in a decided operand without evaluating it would pass
// every case above and answer 0 here.
//
// What is asserted is that the division was raised and the expansion given
// up, not the sentence: this shell's word is `divide` and ours is `division`,
// on every arithmetic failure and not only this one, which is a wording of
// its own and not this axis — docs/spec/ash.md records it as the third of the
// things this dialect cannot yet say.
func TestAFailureInADecidedOperandIsRaised(t *testing.T) {
	out, st := run(t, `echo "v=$((0 && (1/0)))"; echo "after st=$?"`)
	if !strings.Contains(out, "zero") || st == 0 || strings.Contains(out, "v=0") {
		t.Errorf("gave %q at %d, want the division raised and the expansion abandoned", out, st)
	}
}

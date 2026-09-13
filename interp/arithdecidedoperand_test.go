// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The operand of `&&` or `||` whose value can no longer change the answer.
//
// One shell in the panel runs it anyway, so an assignment written there takes
// effect; the rest skip it. What the axis cannot be about is the *value* — `0
// && anything` is 0 and `1 || anything` is 1 under either reading, so the
// number is the operator's and the whole disagreement is about effects.
//
// The tests name the axis and never a shell, which is the rule for this
// package: what a particular shell answers is asserted in its own dialect.

// decided runs a snippet with the axis at a chosen answer, with the
// increment operators available so that both kinds of side effect are
// reachable from one helper.
func decided(t *testing.T, a Answer, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src,
		func(d *syntax.Dialect) { d.ArithIncDec = true },
		func(r *Runner) {
			s := *r.Semantics
			s.ArithShortCircuitEvaluatesTheRightOperand = a
			r.Semantics = &s
		})
}

// TestADecidedOperandIsRunOnlyWhereTheAxisSaysSo is the axis itself, at both
// values, through every shape the panel was measured in.
func TestADecidedOperandIsRunOnlyWhereTheAxisSaysSo(t *testing.T) {
	for _, c := range []struct {
		name, src, runs, skips string
	}{
		{
			name:  "an assignment in the right operand of a false `&&`",
			src:   `x=0; : $((0 && (x = 9))); echo "x=$x"`,
			runs:  "x=9\n",
			skips: "x=0\n",
		},
		{
			name:  "an assignment in the right operand of a true `||`",
			src:   `y=0; : $((1 || (y = 8))); echo "y=$y"`,
			runs:  "y=8\n",
			skips: "y=0\n",
			// The mirror, because an implementation that answered the axis
			// at one operator and not the other would pass the row above.
		},
		{
			name:  "an increment rather than an assignment",
			src:   `x=0; : $((0 && (x++))); echo "x=$x"`,
			runs:  "x=1\n",
			skips: "x=0\n",
			// The axis is about the operand being evaluated, so the other
			// side effect arithmetic has moves with the first.
		},
		{
			name:  "a decrement written in front of its name",
			src:   `x=5; : $((0 && (--x))); echo "x=$x"`,
			runs:  "x=4\n",
			skips: "x=5\n",
		},
		{
			name:  "an assignment two levels inside the operand",
			src:   `x=0; y=0; : $((0 && (1 && (x = 1)) && (y = 2))); echo "x=$x y=$y"`,
			runs:  "x=1 y=2\n",
			skips: "x=0 y=0\n",
			// Nesting is not a bound: an implementation that skipped only
			// the operand it was looking at would leave x unset and y at 2,
			// which no single-level case can tell from either answer.
		},
		{
			name:  "an assignment reached through a conditional inside the operand",
			src:   `x=0; : $((0 && (1 ? (x = 9) : 0))); echo "x=$x"`,
			runs:  "x=9\n",
			skips: "x=0\n",
			// The conditional short-circuits under both answers, but it is
			// *inside* an operand that one of them runs, so its taken arm
			// is reached there.
		},
		{
			name:  "a name the operand would bring into existence",
			src:   `unset q; : $((0 && (q = 9))); echo "q=${q-UNSET}"`,
			runs:  "q=9\n",
			skips: "q=UNSET\n",
			// Set-ness and not only value: the operand that runs creates the
			// name, so a reader asking `${q-…}` can see the difference even
			// where the value would have been the same.
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, st := decided(t, Yes, c.src); out != c.runs || st != 0 {
				t.Errorf("Yes: %s gave %q at %d, want %q at 0", c.src, out, st, c.runs)
			}
			if out, st := decided(t, No, c.src); out != c.skips || st != 0 {
				t.Errorf("No: %s gave %q at %d, want %q at 0", c.src, out, st, c.skips)
			}
		})
	}
}

// TestTheValueIsTheOperatorsUnderEitherAnswer is why this is one question and
// not two.
//
// The expression's number cannot discriminate — it is 0 for a false `&&` and 1
// for a true `||` however the operand is treated — so an implementation graded
// on the value alone would score both answers correct and the divergence would
// stay invisible. That is exactly how it stayed invisible until #2605.
func TestTheValueIsTheOperatorsUnderEitherAnswer(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`x=0; echo "v=$((0 && (x = 9)))"`, "v=0\n"},
		{`x=0; echo "v=$((7 || (x = 9)))"`, "v=1\n"},
		{`x=0; echo "v=$((0 && (x++)))"`, "v=0\n"},
	} {
		for _, a := range []Answer{Yes, No} {
			if out, st := decided(t, a, c.src); out != c.want || st != 0 {
				t.Errorf("%v: %s gave %q at %d, want %q at 0", a, c.src, out, st, c.want)
			}
		}
	}
}

// TestAnOperandTheLeftSideDidNotDecideIsRunUnderEitherAnswer is the control.
//
// Without it, an implementation that had simply lost short-circuiting reads
// exactly like one answering the axis Yes: both leave x at 9. What the axis
// moves is the *decided* operand, and this is the case that says so.
func TestAnOperandTheLeftSideDidNotDecideIsRunUnderEitherAnswer(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`x=0; : $((1 && (x = 9))); echo "x=$x"`, "x=9\n"},
		{`x=0; : $((0 || (x = 8))); echo "x=$x"`, "x=8\n"},
	} {
		for _, a := range []Answer{Yes, No} {
			if out, st := decided(t, a, c.src); out != c.want || st != 0 {
				t.Errorf("%v: %s gave %q at %d, want %q at 0", a, c.src, out, st, c.want)
			}
		}
	}
}

// TestAConditionalDropsItsArmUnderEitherAnswer is the second control, and the
// one that bounds the axis.
//
// The conditional is the other short-circuiting operator, and it is *not* on
// this axis: the arm it did not take is not evaluated under either answer, and
// an `&&` written inside that arm never runs either. An axis implemented as
// "this vector evaluates everything" would move all three of these.
func TestAConditionalDropsItsArmUnderEitherAnswer(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`w=5; echo "v=$((0 ? (w = 1) : 2)) w=$w"`, "v=2 w=5\n"},
		{`w=5; echo "v=$((1 ? 2 : (w = 1))) w=$w"`, "v=2 w=5\n"},
		{`x=0; echo "v=$((0 ? (0 && (x = 9)) : 4)) x=$x"`, "v=4 x=0\n"},
	} {
		for _, a := range []Answer{Yes, No} {
			if out, st := decided(t, a, c.src); out != c.want || st != 0 {
				t.Errorf("%v: %s gave %q at %d, want %q at 0", a, c.src, out, st, c.want)
			}
		}
	}
}

// TestAFailureInADecidedOperandFollowsTheAxis is what says the operand is
// evaluated rather than merely assigned into.
//
// A division by zero cannot be reached by writing a value anywhere, so a shell
// that raises it has run the operand. An implementation that special-cased
// assignment — walking the operand for stores and performing them — would pass
// every case above and fail this one.
func TestAFailureInADecidedOperandFollowsTheAxis(t *testing.T) {
	const src = `echo "v=$((0 && (1/0)))"; echo "after st=$?"`
	if out, _ := decided(t, No, src); out != "v=0\nafter st=0\n" {
		t.Errorf("No: gave %q, want the expression to answer 0 with nothing raised", out)
	}
	out, st := decided(t, Yes, src)
	if strings.Contains(out, "v=0") || st == 0 {
		t.Errorf("Yes: gave %q at %d, want the division to be raised and the expansion to fail", out, st)
	}
}

// TestOrdinaryShortCircuitingAsksNothing pins where the axis is consulted.
//
// `ask` on an unanswered axis refuses and sets status 2, so an axis read at
// the top of the operator would make every `$((a && b))` in the language fail
// under a vector that has not chosen — including every test whose answer set
// predates the axis. The two readings agree on an operand that leaves nothing
// behind, so there is nothing to ask about one.
//
// It fails loudly if a later change hoists the ask, which is the whole reason
// it is written down.
func TestOrdinaryShortCircuitingAsksNothing(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`echo "v=$((0 && 1)) w=$((1 || 0))"`, "v=0 w=1\n"},
		{`a=0; b=2; echo "v=$((a && b)) w=$((b || a))"`, "v=0 w=1\n"},
		{`echo "v=$((0 && (2 + 3)))"`, "v=0\n"},
		{`echo "v=$((1 || (2 > 1)))"`, "v=1\n"},
	} {
		out, st := decided(t, Unspecified, c.src)
		if out != c.want || st != 0 {
			t.Errorf("%s gave %q at %d, want %q at 0 and no refusal", c.src, out, st, c.want)
		}
	}
}

// And the other half of the same rule: an operand that *does* leave something
// behind reaches the disagreement, so an unanswered vector refuses it by name
// rather than quietly picking a side.
func TestADecidedOperandWithASideEffectIsRefusedWhenNothingAnswered(t *testing.T) {
	for _, src := range []string{
		`x=0; : $((0 && (x = 9))); echo "x=$x"`,
		`y=0; : $((1 || (y = 8))); echo "y=$y"`,
		`x=0; : $((0 && (x++))); echo "x=$x"`,
		`x=0; y=0; : $((0 && (1 && (x = 1)))); echo "x=$x"`,
	} {
		out, _ := decided(t, Unspecified, src)
		if !strings.Contains(out, "the shells disagree here and no dialect was chosen") {
			t.Errorf("%s gave %q, want the unanswered axis to be named", src, out)
		}
		if !strings.Contains(out, "the right operand of `&&` or `||`") {
			t.Errorf("%s gave %q, want the refusal to say which axis", src, out)
		}
	}
}

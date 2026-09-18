// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Semantics.ArithRecursionBound, both answers.
//
// What stops a name being resolved through its own value is two questions
// wearing one name, and the panel answers them differently: a **depth**
// refuses a chain of distinct names that does end, and a **cycle** follows it
// however long it is and refuses only a name that comes back on itself. The
// two agree on a real loop and disagree on every terminating chain, which is
// why every row below is one or the other rather than a length.

// boundSem answers the bound and the lookup above it, and nothing else.
func boundSem(p ArithRecursionBoundPolicy) Semantics {
	s := testSemantics()
	s.ArithNameValueRecurses = Yes
	s.ArithRecursionBound = p
	return s
}

// evaluating is boundSem with the right operand of a short-circuited `&&`
// evaluated too, which is the other reading of #2605's axis — the one that
// reaches a loop written where nothing needs its value.
func evaluating(p ArithRecursionBoundPolicy) Semantics {
	s := boundSem(p)
	s.ArithShortCircuitEvaluatesTheRightOperand = Yes
	return s
}

// chain writes n names, each holding the next, with the last holding a number:
// `v0=v1`, `v1=v2`, … `vN=7`. Written out rather than built with `eval` so the
// snippet asks about the bound and not about anything `eval` answers.
func chain(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "v%d=v%d; ", i, i+1)
	}
	fmt.Fprintf(&b, "v%d=7; echo \"v=$(( v0 ))\"", n)
	return b.String()
}

func TestAChainOfNamesIsBoundedByADepthOrByACycle(t *testing.T) {
	t.Run("a short chain is its number under both readings", func(t *testing.T) {
		// The control. A chain the frame count does not reach is a value
		// either way, so a row further down that differs is about the bound
		// and not about whether the lookup happens at all.
		for _, p := range []ArithRecursionBoundPolicy{ArithRecursionBoundedByDepth, ArithRecursionBoundedByACycle} {
			out, st := run(t, chain(3), withSem(boundSem(p)))
			if out != "v=7\n" || st != 0 {
				t.Errorf("%v: = %q status %d, want \"v=7\\n\" and 0", p, out, st)
			}
		}
	})

	t.Run("a long chain is refused by a depth", func(t *testing.T) {
		out, st := run(t, chain(60), withSem(boundSem(ArithRecursionBoundedByDepth)))
		if st == 0 {
			t.Errorf("= %q status %d, want a failing status", out, st)
		}
		if strings.Contains(out, "v=7") {
			t.Errorf("= %q, want no value — the frame count stopped it", out)
		}
	})

	t.Run("and followed to its end by a cycle", func(t *testing.T) {
		// The row the two readings part on. A chain of sixty distinct names
		// ends in a number, so a shell looking for a *name that repeats*
		// never finds one and answers 7.
		out, st := run(t, chain(60), withSem(boundSem(ArithRecursionBoundedByACycle)))
		if out != "v=7\n" || st != 0 {
			t.Errorf("= %q status %d, want \"v=7\\n\" and 0", out, st)
		}
	})

	t.Run("a chain past three hundred is still followed", func(t *testing.T) {
		// Far past any frame count a depth reading could plausibly hold, so
		// the row cannot pass by the bound merely being large.
		out, st := run(t, chain(300), withSem(boundSem(ArithRecursionBoundedByACycle)))
		if out != "v=7\n" || st != 0 {
			t.Errorf("= %q status %d, want \"v=7\\n\" and 0", out, st)
		}
	})
}

// A loop is refused under both readings, which is what says the cycle reading
// is a different bound rather than no bound at all.
func TestALoopIsRefusedUnderBothBounds(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a name that points at itself", `x=x; echo "v=$(( x ))"`},
		{"two names that point at each other", `a=b; b=a; echo "v=$(( a+1 ))"`},
		{"a loop through an expression", `a=b+1; b=a; echo "v=$(( a ))"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, p := range []ArithRecursionBoundPolicy{ArithRecursionBoundedByDepth, ArithRecursionBoundedByACycle} {
				out, st := run(t, tc.src, withSem(boundSem(p)))
				if st == 0 {
					t.Errorf("%v: = %q status %d, want a failing status", p, out, st)
				}
				if strings.Contains(out, "v=") {
					t.Errorf("%v: = %q, want no value", p, out)
				}
			}
		})
	}
}

// A loop written where nothing needs its value is still reached, in the
// reading that evaluates a short-circuited operand — so the two axes compose
// rather than one hiding the other.
func TestALoopBehindAShortCircuitIsStillBounded(t *testing.T) {
	for _, p := range []ArithRecursionBoundPolicy{ArithRecursionBoundedByDepth, ArithRecursionBoundedByACycle} {
		out, st := run(t, `x=x; echo "v=$(( 0 && x ))"`, withSem(evaluating(p)))
		if st == 0 || strings.Contains(out, "v=") {
			t.Errorf("%v: = %q status %d, want a failure and no value", p, out, st)
		}
	}
	// And the control: with the operand left alone the expression is 0, so
	// the row above is about the loop being reached rather than about the
	// bound firing on something nothing evaluated.
	out, st := run(t, `x=x; echo "v=$(( 0 && x ))"`, withSem(boundSem(ArithRecursionBoundedByACycle)))
	if out != "v=0\n" || st != 0 {
		t.Errorf("= %q status %d, want \"v=0\\n\" and 0", out, st)
	}
}

// The cycle reading blames nothing, where a depth blames a name.
//
// A verb in the wording is what says which: the sentence one column writes
// carries no name at all, so a reading that handed the name to the format
// would put one there the moment a dialect wrote `%[1]s`.
func TestTheCycleBoundNamesNothing(t *testing.T) {
	const src = `a=b; b=a; echo "v=$(( a ))"`
	worded := func(p ArithRecursionBoundPolicy) func(*Runner) {
		return func(r *Runner) {
			s := boundSem(p)
			r.Semantics = &s
			r.Diagnostics = &Diagnostics{ArithRecursionLimit: "stopped at [%[1]s]"}
		}
	}
	out, _ := run(t, src, worded(ArithRecursionBoundedByDepth))
	if !strings.Contains(out, "stopped at [b]") {
		t.Errorf("a depth: got %q, want the name it stopped on", out)
	}
	out, _ = run(t, src, worded(ArithRecursionBoundedByACycle))
	if !strings.Contains(out, "stopped at []") {
		t.Errorf("a cycle: got %q, want nothing blamed", out)
	}
}

// An unanswered bound refuses rather than recurring, and says so once.
//
// The once matters as much as the refusal: the bound is consulted at every
// frame, so a reading that carried on past an unanswered axis would write the
// same complaint as many times as the chain is deep.
func TestAnUnansweredRecursionBoundIsRefusedOnce(t *testing.T) {
	out, st := run(t, `x=x; echo "v=$(( x ))"`, withSem(boundSem(ArithRecursionBoundUnspecified)))
	if st == 0 {
		t.Errorf("= %q status %d, want a failing status", out, st)
	}
	if n := strings.Count(out, "no dialect"); n != 1 {
		t.Errorf("= %q, want the refusal named once, got %d", out, n)
	}
}

// A name written twice in one expression is not a cycle.
//
// The chain is popped on every way out, so two operands naming the same
// variable are two independent lookups — which a set that only ever grew
// would report as a loop, silently turning `$(( v+v ))` into a fatal error.
func TestTwoOperandsNamingOneVariableAreNotALoop(t *testing.T) {
	out, st := run(t, `y=5; v=y; echo "v=$(( v+v ))"`, withSem(boundSem(ArithRecursionBoundedByACycle)))
	if out != "v=10\n" || st != 0 {
		t.Errorf("= %q status %d, want \"v=10\\n\" and 0", out, st)
	}
}

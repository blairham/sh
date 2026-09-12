// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `unset "a[lo,hi]"` where the dialect reads the comma as a range. What the
// span becomes is UnsetArraySpan, which was already answered; what is new is
// that a span can be written at all, and that the below-the-first-element
// refusal belongs to the span rather than to the subscript that starts it.

// rangeUnsetRun answers the reading, the base and the span policy, and runs
// with the grammar an array literal needs.
func rangeUnsetRun(t *testing.T, src string, set func(*Semantics)) (string, int) {
	t.Helper()
	return runGrammar(t, src, nil, func(r *Runner) {
		sem := *r.Semantics
		sem.UnsetTakesASubscript = Yes
		sem.SubscriptCommaIsARange = Yes
		sem.ArrayBaseIsZero = No
		sem.UnsetArraySpan = UnsetArraySpanLeavesOneEmptyElement
		if set != nil {
			set(&sem)
		}
		r.Semantics = &sem
	})
}

const showArray = `; echo -n "st=$? "; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`

func TestUnsettingARangeReplacesTheSpanWithOneEmptyElement(t *testing.T) {
	for _, tc := range []struct{ sub, want string }{
		{"1,2", "st=0 [][z] n=2\n"},
		{"1,3", "st=0 [] n=1\n"},
		{"2,3", "st=0 [x][] n=2\n"},
		{"2,-1", "st=0 [x][] n=2\n"},
		{"0,-1", "st=0 [] n=1\n"},
	} {
		out, st := rangeUnsetRun(t, `a=(x y z); unset "a[`+tc.sub+`]"`+showArray, nil)
		if st != 0 || out != tc.want {
			t.Errorf("a[%s]: got %q status %d, want %q", tc.sub, out, st, tc.want)
		}
	}
}

func TestTheBelowTheFirstElementRefusalBelongsToTheSpan(t *testing.T) {
	// A range that begins out of reach and ends inside is not refused: the
	// start is the first element. One that lies wholly out of reach is.
	out, st := rangeUnsetRun(t, `a=(x y z); unset "a[0,1]"`+showArray, nil)
	if st != 0 || out != "st=0 [][y][z] n=3\n" {
		t.Errorf("a[0,1]: got %q status %d, want the start clamped to the first", out, st)
	}
	out, st = rangeUnsetRun(t, `a=(x y z); unset "a[0,0]"`+showArray, nil)
	if st != 0 || out != "sh: unset: [0,0]: bad array subscript\nst=1 [x][y][z] n=3\n" {
		t.Errorf("a[0,0]: got %q status %d, want the span refused and the array whole", out, st)
	}
	// And the single subscript is that span with one end: the same rule, so
	// `a[0]` is refused for the same reason `a[0,0]` is.
	out, st = rangeUnsetRun(t, `a=(x y z); unset "a[0]"`+showArray, nil)
	if st != 0 || out != "sh: unset: [0]: bad array subscript\nst=1 [x][y][z] n=3\n" {
		t.Errorf("a[0]: got %q status %d, want the same refusal", out, st)
	}
	// Where the first element is 0 nothing is out of reach, and the same
	// numerals name a span that is there.
	out, st = rangeUnsetRun(t, `a=(x y z); unset "a[0,0]"`+showArray,
		func(s *Semantics) { s.ArrayBaseIsZero = Yes })
	if st != 0 || out != "st=0 [][y][z] n=3\n" {
		t.Errorf("base 0: got %q status %d, want the first element blanked", out, st)
	}
}

func TestAReversedRangeInsertsAnEmptyElement(t *testing.T) {
	// A span with nothing in it still becomes one empty element, so the array
	// gains one where the span would have begun. It is the strongest evidence
	// that this reading is a replacement and not a removal.
	for _, tc := range []struct{ sub, want string }{
		{"1,0", "st=0 [][x][y][z] n=4\n"},
		{"2,1", "st=0 [x][][y][z] n=4\n"},
		{"3,2", "st=0 [x][y][][z] n=4\n"},
		{"0,-4", "st=0 [][x][y][z] n=4\n"},
		// Reversed by more than one: the span is still empty *at the start*
		// and nothing between the two ends is disturbed, which a tail taken
		// from the end alone would have duplicated.
		{"3,1", "st=0 [x][y][][z] n=4\n"},
		{"-1,-3", "st=0 [x][y][][z] n=4\n"},
	} {
		out, st := rangeUnsetRun(t, `a=(x y z); unset "a[`+tc.sub+`]"`+showArray, nil)
		if st != 0 || out != tc.want {
			t.Errorf("a[%s]: got %q status %d, want %q", tc.sub, out, st, tc.want)
		}
	}
}

func TestARangeStartingPastTheEndDoesNothing(t *testing.T) {
	// The other end of the same rule: an end past the last is the last, but a
	// start past the last has nothing to replace and nothing to stand in
	// front of.
	out, st := rangeUnsetRun(t, `a=(x y z); unset "a[4,5]"`+showArray, nil)
	if st != 0 || out != "st=0 [x][y][z] n=3\n" {
		t.Errorf("a[4,5]: got %q status %d, want nothing done", out, st)
	}
	out, st = rangeUnsetRun(t, `a=(x y z); unset "a[3,4]"`+showArray, nil)
	if st != 0 || out != "st=0 [x][y][] n=3\n" {
		t.Errorf("a[3,4]: got %q status %d, want the end clamped to the last", out, st)
	}
	// An array with no elements has nowhere for a span to begin either.
	out, st = rangeUnsetRun(t, `a=(); unset "a[1,2]"`+showArray, nil)
	if st != 0 || out != "st=0 [] n=0\n" {
		t.Errorf("empty: got %q status %d, want nothing gained", out, st)
	}
}

func TestARangesNegativeStartFollowsTheSingleSubscriptsRule(t *testing.T) {
	// Only the last element answers to a negative subscript where the span is
	// blanked, and a range's start is no exception.
	out, st := rangeUnsetRun(t, `a=(x y z); unset "a[-1,-1]"`+showArray, nil)
	if st != 0 || out != "st=0 [x][y][] n=3\n" {
		t.Errorf("a[-1,-1]: got %q status %d, want the last element blanked", out, st)
	}
	for _, sub := range []string{"-2,-1", "-3,3", "-9,2"} {
		out, st := rangeUnsetRun(t, `a=(x y z); unset "a[`+sub+`]"`+showArray, nil)
		if st != 0 || out != "st=0 [x][y][z] n=3\n" {
			t.Errorf("a[%s]: got %q status %d, want the array left whole", sub, out, st)
		}
	}
}

func TestARangeOverAStringNamesCharacters(t *testing.T) {
	for _, tc := range []struct{ sub, want string }{
		{"2,3", "st=0 [hlo]\n"},
		{"1,2", "st=0 [llo]\n"},
		{"-2,-1", "st=0 [hel]\n"},
		{"0,-1", "st=0 []\n"},
		{"2,9", "st=0 [h]\n"},
		{"3,2", "st=0 [hello]\n"},
		{"9,9", "st=0 [hello]\n"},
	} {
		out, st := rangeUnsetRun(t, `a=hello; unset "a[`+tc.sub+`]"; echo "st=$? [${a-UNSET}]"`,
			func(s *Semantics) { s.ScalarSubscriptIsACharacter = Yes })
		if st != 0 || out != tc.want {
			t.Errorf("a[%s]: got %q status %d, want %q", tc.sub, out, st, tc.want)
		}
	}
	// The same refusal, reached through a string.
	out, st := rangeUnsetRun(t, `a=hello; unset "a[0,0]"; echo "st=$? [${a-UNSET}]"`,
		func(s *Semantics) { s.ScalarSubscriptIsACharacter = Yes })
	if st != 0 || out != "sh: unset: [0,0]: bad array subscript\nst=1 [hello]\n" {
		t.Errorf("got %q status %d, want the span refused", out, st)
	}
}

func TestARangeOverAScalarReadAsAnElement(t *testing.T) {
	// No shell measured reads both a range and a scalar-as-element, so this
	// is the two readings composed: a span that reaches the one element a
	// scalar is takes the whole name away, and one that does not is the
	// question every other subscript on a scalar asks.
	out, st := rangeUnsetRun(t, `a=hello; unset "a[0,1]"; echo "st=$? [${a-UNSET}]"`,
		func(s *Semantics) {
			s.ScalarSubscriptIsACharacter = No
			s.ArrayBaseIsZero = Yes
			s.UnsetSubscriptOnAScalarIsAnError = Yes
		})
	if st != 0 || out != "st=0 [UNSET]\n" {
		t.Errorf("reaching: got %q status %d, want the name taken away", out, st)
	}
	out, st = rangeUnsetRun(t, `a=hello; unset "a[2,3]"; echo "st=$? [${a-UNSET}]"`,
		func(s *Semantics) {
			s.ScalarSubscriptIsACharacter = No
			s.ArrayBaseIsZero = Yes
			s.UnsetSubscriptOnAScalarIsAnError = Yes
		})
	if st != 0 || out != "sh: unset: a: not an array variable\nst=1 [hello]\n" {
		t.Errorf("out of reach: got %q status %d, want the scalar refusal", out, st)
	}
	// And a span with nothing in it reaches no element either, however close
	// to the one a scalar is it begins: an empty span is not the element it
	// stands in front of.
	out, st = rangeUnsetRun(t, `a=hello; unset "a[0,-2]"; echo "st=$? [${a-UNSET}]"`,
		func(s *Semantics) {
			s.ScalarSubscriptIsACharacter = No
			s.ArrayBaseIsZero = Yes
			s.UnsetSubscriptOnAScalarIsAnError = Yes
		})
	if st != 0 || out != "sh: unset: a: not an array variable\nst=1 [hello]\n" {
		t.Errorf("empty span: got %q status %d, want the scalar refusal", out, st)
	}
}

func TestARangeRemovesTheSpanWhereThatIsTheAnswer(t *testing.T) {
	// The other span policy read over a range. No dialect answers both this
	// way and reads ranges, so it is the two answers composed rather than a
	// column of its own — and it is the reading the policy's name states.
	out, st := rangeUnsetRun(t, `a=(x y z); unset "a[1,2]"`+showArray,
		func(s *Semantics) { s.UnsetArraySpan = UnsetArraySpanRemovesTheElements })
	if st != 0 || out != "st=0 [z] n=1\n" {
		t.Errorf("got %q status %d, want the span removed and nothing left in its place", out, st)
	}
}

func TestAPairWhoseEndsAreTheSameSubscriptAsksNothing(t *testing.T) {
	// It is that subscript under either reading, so the single path answers
	// and the axis is not asked — which is what lets a runner with no dialect
	// answer `unset "a[2,2]"` at all.
	out, st := runGrammar(t, `a=(x y z); unset "a[2,2]"`+showArray, nil, func(r *Runner) {
		sem := *r.Semantics
		sem.UnsetTakesASubscript = Yes
		sem.SubscriptCommaIsARange = Unspecified
		sem.ArrayBaseIsZero = No
		sem.UnsetArraySpan = UnsetArraySpanLeavesOneEmptyElement
		r.Semantics = &sem
	})
	if st != 0 || out != "st=0 [x][][z] n=3\n" {
		t.Errorf("got %q status %d, want the one element blanked with no question asked", out, st)
	}
	// Where they differ, the unanswered axis is refused rather than guessed.
	if _, st := runGrammar(t, `a=(x y z); unset "a[1,2]"`, nil, func(r *Runner) {
		sem := *r.Semantics
		sem.UnsetTakesASubscript = Yes
		sem.SubscriptCommaIsARange = Unspecified
		sem.ArrayBaseIsZero = No
		sem.UnsetArraySpan = UnsetArraySpanLeavesOneEmptyElement
		r.Semantics = &sem
	}); st != 2 {
		t.Errorf("status %d, want the unanswered axis refused", st)
	}
}

func TestARangeEndpointThatWillNotEvaluateIsReported(t *testing.T) {
	// Each end is an expression and each is reported where it fails, at both
	// ends rather than at whichever one is read first. What is *done* about
	// it differs by end, which is the pair of tests below; here only the
	// report is asserted, and the start is the end that also leaves the array
	// alone.
	for _, sub := range []string{"x+,2", "1,x+"} {
		out, _ := rangeUnsetRun(t, `a=(x y z); unset "a[`+sub+`]"`+showArray,
			func(s *Semantics) { s.BadSubscriptToUnsetFatal = No })
		if !strings.HasPrefix(out, "sh: x+: operand expected\nst=1 ") {
			t.Errorf("a[%s]: got %q, want the expression reported at 1", sub, out)
		}
	}
}

// An endpoint that is no expression is answered differently at the two ends,
// and the asymmetry is the whole of it. Measured 2026-09-12 on the one shell
// that reads a comma as a range: a bad **start** is reported and nothing is
// done, and a bad **end** is reported and the range is then acted on with the
// end carrying 0 (#1001).
//
// Every want below is what the same range with a written 0 produces, which is
// what says the 0 is a value and not a rule of its own: `[1,x+]` is `[1,0]`,
// and a reversed range in this policy leaves an empty element where the span
// would have begun.
func TestARangeEndThatWillNotEvaluateCarriesZero(t *testing.T) {
	const bad = "sh: x+: operand expected\n"
	for _, tc := range []struct{ sub, want string }{
		{"1,x+", bad + "st=1 [][x][y][z] n=4\n"},
		{"2,x+", bad + "st=1 [x][][y][z] n=4\n"},
		{"3,x+", bad + "st=1 [x][y][][z] n=4\n"},
		{"-1,x+", bad + "st=1 [x][y][][z] n=4\n"},
		// A start past the last element has nothing to stand in front of, so
		// the carried 0 changes nothing — the same as a written `[4,0]`.
		{"4,x+", bad + "st=1 [x][y][z] n=3\n"},
	} {
		out, st := rangeUnsetRun(t, `a=(x y z); unset "a[`+tc.sub+`]"`+showArray,
			func(s *Semantics) { s.BadSubscriptToUnsetFatal = No })
		if st != 0 || out != tc.want {
			t.Errorf("a[%s]: got %q status %d, want %q", tc.sub, out, st, tc.want)
		}
	}
}

// The start carries nothing forward: it is reported and the array is left
// alone, which is the single subscript's rule. Asserted beside the end's
// answer, because a fix that made the two agree would look right on either
// one alone.
func TestARangeStartThatWillNotEvaluateDoesNothing(t *testing.T) {
	const bad = "sh: x+: operand expected\n"
	for _, sub := range []string{"x+,2", "x+,y+"} {
		out, st := rangeUnsetRun(t, `a=(x y z); unset "a[`+sub+`]"`+showArray,
			func(s *Semantics) { s.BadSubscriptToUnsetFatal = No })
		if st != 0 || out != bad+"st=1 [x][y][z] n=3\n" {
			t.Errorf("a[%s]: got %q status %d, want the array whole", sub, out, st)
		}
	}
}

// One failed subscript, one sentence. `[0,x+]` carries 0 and becomes `[0,0]`,
// a span wholly below the first element — and the shell writes the math error
// *alone*, where a written `a[0,0]` also writes the bad-subscript refusal.
//
// This is the row that says the carried 0 is not simply substituted and then
// forgotten: an implementation that reported and fell into the ordinary path
// writes two diagnostics for one mistake.
func TestASecondComplaintIsSwallowedOnceTheEndpointIsReported(t *testing.T) {
	out, st := rangeUnsetRun(t, `a=(x y z); unset "a[0,x+]"`+showArray,
		func(s *Semantics) { s.BadSubscriptToUnsetFatal = No })
	if st != 0 || out != "sh: x+: operand expected\nst=1 [x][y][z] n=3\n" {
		t.Errorf("a[0,x+]: got %q status %d, want one sentence and the array whole", out, st)
	}
}

func TestASubscriptThatIsNotAPairAsksTheRangeAxisNothing(t *testing.T) {
	// Even one that will not evaluate: it is not a range, so the failure is
	// the subscript's own and not an unanswered axis. Read with the axis
	// unanswered, which is the only way to tell the two reports apart.
	out, st := runGrammar(t, `a=(x y z); unset "a[x+]"`+showArray, nil, func(r *Runner) {
		sem := *r.Semantics
		sem.UnsetTakesASubscript = Yes
		sem.SubscriptCommaIsARange = Unspecified
		sem.ArrayBaseIsZero = No
		sem.UnsetArraySpan = UnsetArraySpanLeavesOneEmptyElement
		sem.BadSubscriptToUnsetFatal = No
		r.Semantics = &sem
	})
	if st != 0 || out != "sh: x+: operand expected\nst=1 [x][y][z] n=3\n" {
		t.Errorf("got %q status %d, want the subscript reported rather than the axis", out, st)
	}
}

func TestARangeOverANameHoldingNothingIsQuiet(t *testing.T) {
	// No element and no character for a span to reach, whichever reading is
	// in force and whatever the dialect does about a subscript on a scalar.
	for _, sub := range []string{"0,1", "2,3", "1,0"} {
		out, st := rangeUnsetRun(t, `unset b; unset "b[`+sub+`]"; echo "st=$? [${b-UNSET}]"`,
			func(s *Semantics) {
				s.ScalarSubscriptIsACharacter = No
				s.ArrayBaseIsZero = Yes
				s.UnsetSubscriptOnAScalarIsAnError = Yes
			})
		if st != 0 || out != "st=0 [UNSET]\n" {
			t.Errorf("b[%s]: got %q status %d, want nothing said and nothing done", sub, out, st)
		}
	}
}

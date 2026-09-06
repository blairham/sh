// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
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

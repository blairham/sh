// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

// The four quantifiers a pattern group may carry, as bounds.
//
// Measured 2026-09-27 over `[[ $s == p ]]` with s empty, `a` and `aa`, on
// `/opt/homebrew/bin/bash` 5.3.20 with `shopt -s extglob` and on `/bin/ksh`
// `Version AJM 93u+ 2012-08-01`; the two are unanimous on all twelve cells
// and the table is repeated in boundOf's own comment.
func TestAQuantifierIsABound(t *testing.T) {
	for _, tc := range []struct {
		quant byte
		want  repeatBound
	}{
		{'@', repeatBound{1, 1}},
		{'?', repeatBound{0, 1}},
		{'+', repeatBound{1, -1}},
		{'*', repeatBound{0, -1}},
		// The bare group one dialect has, which is exactly one — `(a)b`
		// matches `ab` and not `aab` on zsh 5.9.2, the same answer `@(a)b`
		// gives in the two shells above.
		{0, repeatBound{1, 1}},
		// And the complement, which is here only to say that asking is
		// harmless: its own branch answers it before any bound is read, so
		// the value is never used and must not be mistaken for a claim that
		// a negation repeats once.
		{'!', repeatBound{1, 1}},
	} {
		if got := boundOf(tc.quant); got != tc.want {
			t.Errorf("boundOf(%q) = %v, want %v", tc.quant, got, tc.want)
		}
	}
}

// **The half no spelling reaches yet**, tested here because nothing else can.
//
// [repeatBound.afterOne] is what makes a finite ceiling terminate, and today
// every bound that reaches it is `+` or `*` — floor at most one, no ceiling at
// all — so neither the floor it spends nor the ceiling it spends changes any
// answer a pattern can ask for. Two mutants prove that rather than assert it:
// with `afterOne` leaving the floor alone, and again with it leaving the
// ceiling alone, `cmd/ksh` answered all twelve rows of the quantifier table
// identically. Four other mutants — no ceiling anywhere, `mayRepeat` dropping
// the unbounded case, `mayStopHere` never, and `?` as exactly one — were each
// killed by two or three of those rows.
//
// So the rows below are the only thing standing between this function and a
// change nothing would notice until a count with a real ceiling exists. They
// are written at the values such a count produces, not at the values `+` and
// `*` produce.
func TestABoundCountsDownAsTheGroupRepeats(t *testing.T) {
	for _, tc := range []struct {
		name      string
		b         repeatBound
		stopHere  bool
		mayRepeat bool
		after     repeatBound
	}{
		{"exactly one", repeatBound{1, 1}, false, false, repeatBound{0, 0}},
		{"nought or one", repeatBound{0, 1}, true, false, repeatBound{0, 0}},
		{"one or more", repeatBound{1, -1}, false, true, repeatBound{0, -1}},
		{"any number", repeatBound{0, -1}, true, true, repeatBound{0, -1}},
		// The values a written count produces and a quantifier cannot.
		{"two or three", repeatBound{2, 3}, false, true, repeatBound{1, 2}},
		{"the last of three", repeatBound{0, 1}, true, false, repeatBound{0, 0}},
		{"exactly two", repeatBound{2, 2}, false, true, repeatBound{1, 1}},
		{"two or more", repeatBound{2, -1}, false, true, repeatBound{1, -1}},
		// A floor already spent stays spent rather than going negative,
		// which is what lets a group that has had all it needs keep going.
		{"spent", repeatBound{0, 0}, true, false, repeatBound{0, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.b.mayStopHere(); got != tc.stopHere {
				t.Errorf("%v.mayStopHere() = %v, want %v", tc.b, got, tc.stopHere)
			}
			if got := tc.b.mayRepeat(); got != tc.mayRepeat {
				t.Errorf("%v.mayRepeat() = %v, want %v", tc.b, got, tc.mayRepeat)
			}
			if got := tc.b.afterOne(); got != tc.after {
				t.Errorf("%v.afterOne() = %v, want %v", tc.b, got, tc.after)
			}
		})
	}
}

// And a bound is a value, so a round of the group that fails leaves the
// caller's own untouched.
//
// It matters because [matchGroupTimes] tries every arm against every split
// with the same bound in hand: a bound that counted down in place would spend
// the arm that failed as well as the one that matched, and the pattern would
// then refuse a subject for want of repetitions it had never used.
func TestSpendingABoundLeavesTheCallersAlone(t *testing.T) {
	b := repeatBound{2, 3}
	_ = b.afterOne()
	_ = b.afterOne()
	if b != (repeatBound{2, 3}) {
		t.Errorf("the bound moved to %v after two rounds that were thrown away", b)
	}
}

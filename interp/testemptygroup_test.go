// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// A group with nothing between its parentheses, on the axis that reads one as
// a false expression rather than as a list that ran out of words (#3687).
//
// The rows are written as *statuses* rather than as sentences because that is
// what separates this from the rest of the disagreements about groups: every
// other row of the measurement differs only in the wording, at status 2 on
// both sides, and no script can see it. Here a script reading `$?` gets 1
// where the other answer writes a sentence and exits 2.
//
// Measured 2026-09-18, script files under `env -i PATH=/usr/bin:/bin LC_ALL=C`
// with standard input on /dev/null — dash 0.5.12, bash 5.3.20, zsh 5.9.2,
// ksh93u+ 2012-08-01 and BusyBox v1.37.0 ash in the pinned image. One column
// answers the first way and five the second; see
// Semantics.TestEmptyGroupIsFalse for the rows.
//
// Both readings of `test` are exercised, because the shape arrives by two
// routes that share no code: the argument counts POSIX specifies take the
// two-word and four-word lists, and the grammar takes everything longer. A
// fix in one of them leaves the other refusing, which is what the connective
// rows below are for.
func TestAnEmptyGroupIsAValueOnlyWhereTheAxisSaysSo(t *testing.T) {
	run := func(t *testing.T, empty Answer, src string) (string, int) {
		t.Helper()
		return optRun(t, func(s *Semantics) {
			s.TestEmptyGroupIsFalse = empty
		}, Diagnostics{}, src)
	}
	for _, tc := range []struct {
		name, src string
		// status where the empty group is a value, and the count each
		// route reaches the shape by.
		want int
	}{
		// The group alone, which the two-word count takes.
		{"the group alone", `[ '(' ')' ]`, 1},
		// Nested, which the four-word count takes and hands back to the
		// two-word one.
		{"a group holding only a group", `[ '(' '(' ')' ')' ]`, 1},
		// Behind a negation, which the three-word count takes: false
		// negated is true, so this row also says the value is a *value*
		// and not a shape the reader skips.
		{"behind a negation", `[ ! '(' ')' ]`, 0},
		// The grammar's own route. `-a` over a false right side is false
		// whatever stood in front of it.
		{"as the right side of an and", `[ x -a '(' ')' ]`, 1},
		// And `-o` over a false left side is whatever is behind it, which
		// is what says the group did not swallow the connective.
		{"as the left side of an or", `[ '(' ')' -o x ]`, 0},
		{"on both sides of an or", `[ '(' ')' -o '(' ')' ]`, 1},
		{"on both sides of an and", `[ '(' ')' -a '(' ')' ]`, 1},
		// Inside a longer expression, where `-a` binds tighter: false and
		// x is false, or y is true.
		{"inside a longer expression", `[ '(' ')' -a x -o y ]`, 0},
		// And under the other name the same builtin answers to, because
		// the reading is the builtin's and not the bracket's.
		{"under the other spelling", `test '(' ')'`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, Yes, tc.src)
			if out != "" || st != tc.want {
				t.Errorf("a value: %s wrote %q at %d, want silence at %d", tc.src, out, st, tc.want)
			}
			// The mutation: the other answer refuses the same words. A
			// status of 2 with something said is what every column that
			// does not read the group this way writes, and a test that
			// only asserted the first half would pass against a reader
			// that had quietly made the group false everywhere.
			out, st = run(t, No, tc.src)
			if out == "" || st != 2 {
				t.Errorf("not a value: %s wrote %q at %d, want a refusal at 2", tc.src, out, st)
			}
		})
	}
}

// And a group that holds something is read the same way under both answers,
// which is what says the axis is about the *empty* group and not about
// grouping.
//
// Measured with the same probes on the same day: `[ ( x ) ]` is 0 and
// `[ ( "" ) ]` is 1 in all six columns, the one that reads an empty group as
// false included.
func TestAGroupThatHoldsSomethingIsReadEitherWay(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		want      int
	}{
		{"a non-empty word in a group", `[ '(' x ')' ]`, 0},
		{"an empty word in a group", `[ '(' "" ')' ]`, 1},
		{"a group inside an and", `[ x -a '(' y ')' ]`, 0},
		{"a comparison in a group", `[ '(' x = x ')' ]`, 0},
		{"a group holding a group", `[ '(' '(' x ')' ')' ]`, 0},
		{"a negation inside a group", `[ '(' ! x ')' ]`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, empty := range []Answer{Yes, No} {
				out, st := optRun(t, func(s *Semantics) {
					s.TestEmptyGroupIsFalse = empty
				}, Diagnostics{}, tc.src)
				if out != "" || st != tc.want {
					t.Errorf("%v: %s wrote %q at %d, want silence at %d", empty, tc.src, out, st, tc.want)
				}
			}
		})
	}
}

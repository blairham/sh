// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// braceFields runs one snippet under a vector that answers
// BraceFanExpandsEachNameOnItsOwn the given way — which is the road the word
// takes — with every other brace axis the rows reach pinned so that nothing
// here is decided by a refusal.
func braceFields(t *testing.T, src string, fan Answer) (string, int) {
	t.Helper()
	out, st := run(t, emptyAltCounter+src+"\n", func(r *Runner) {
		s := *r.Semantics
		s.BraceExpansion = Yes
		s.BraceFanExpandsEachNameOnItsOwn = fan
		s.BraceOutputRereadAsText = No
		s.BraceRescanEntersFailedGroup = No
		s.BraceBodyReadAfterExpansion = No
		s.BraceEmptyAlternativeIsAField = No
		s.BraceStopsFieldSplitting = No
		s.BraceRangeEndpointsExpanded = Yes
		r.Semantics = &s
	})
	return strings.TrimSpace(out), st
}

// Where a word's braces are found: in the word the parse cut, or in the
// fields the word came to.
//
// The two roads are the same axis as whether the word is expanded once or
// once per name, because they are the same fact: a road that finds the braces
// in the fields has already expanded the word, and one that substitutes an
// alternative back into the word has not. See interp/bracefields.go for the
// panel and for what defeats the readings that were tried before this one.
func TestWhereAWordsBracesAreFound(t *testing.T) {
	for _, tc := range []struct{ name, src, once, perName string }{
		// The control: with nothing multi-field in the word the two roads
		// answer the same, which is why a word of literal text never asks.
		{"a group and nothing else", `set -- 1 2; f x{p,q}y`, `2 | [xpy] [xqy]`, `2 | [xpy] [xqy]`},
		{"an empty list joins", `set --; f x{p,q}$@y`, `2 | [xpy] [xqy]`, `2 | [xpy] [xqy]`},

		// A list behind the group. Expanding once lays it into the fields
		// the group left open; expanding per name copies the whole list.
		{
			"a list behind the group", `set -- 1 2; f x{p,q}$@y`,
			`3 | [xp1] [xq1] [2y]`, `4 | [xp1] [2y] [xq1] [2y]`,
		},
		{
			"with nothing behind it", `set -- 1 2; f {p,q}$@`,
			`3 | [p1] [q1] [2]`, `4 | [p1] [2] [q1] [2]`,
		},
		{
			"a list in front of it", `set -- 1 2; f $@{p,q}y`,
			`3 | [1] [2py] [2qy]`, `4 | [1] [2py] [1] [2qy]`,
		},
		{
			"a list on both sides", `set -- 1 2; f $@{p,q}$@`,
			`4 | [1] [2p1] [2q1] [2]`, `6 | [1] [2p1] [2] [1] [2q1] [2]`,
		},
		{
			"two groups and two lists", `set -- 1 2; f x{p,q}$@y{r,s}$@z`,
			`5 | [xp1] [xq1] [2yr1] [2ys1] [2z]`,
			`12 | [xp1] [2yr1] [2z] [xp1] [2ys1] [2z] [xq1] [2yr1] [2z] [xq1] [2ys1] [2z]`,
		},
		{
			"a quoted list is the same question", `set -- 1 2; f x{p,q}"$@"y`,
			`3 | [xp1] [xq1] [2y]`, `4 | [xp1] [2y] [xq1] [2y]`,
		},

		// **The row that decides what the model is.** A list inside the
		// group puts a field boundary between its braces, so on the road
		// that reads the fields there is no group left at all and the braces
		// are in the output. No rule about distributing a group over a word
		// can say that.
		{
			"a list inside the group", `set -- 1 2; f x{p,$@}y`,
			`2 | [x{p,1] [2}y]`, `3 | [xpy] [x1] [2y]`,
		},
		// The control for it: the same written shape with a scalar in the
		// same place, which keeps the group on both roads. It is what says
		// the row above is about the field boundary and not about the shape.
		{
			"a scalar inside the group", `set -- 1; f x{p,$1}y`,
			`2 | [xpy] [x1y]`, `2 | [xpy] [x1y]`,
		},
		{
			"and a group that is only a list", `set -- 1 2; f {$@}`,
			`2 | [{1] [2}]`, `2 | [{1] [2}]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := braceFields(t, tc.src, No); out != tc.once || st != 0 {
				t.Errorf("once: %s = %q status %d, want %q", tc.src, out, st, tc.once)
			}
			if out, st := braceFields(t, tc.src, Yes); out != tc.perName || st != 0 {
				t.Errorf("per name: %s = %q status %d, want %q", tc.src, out, st, tc.perName)
			}
		})
	}
}

// Only the text the script wrote is brace syntax, on the road that finds the
// braces in the fields — which is the half that cannot be got at without
// keeping, beside each field, where its bytes came from.
//
// Every row is the same answer on both roads, which is the point: a road
// that reads a value's braces would answer them differently, and that is a
// question of its own (#4797) rather than this one.
func TestOnlyWrittenTextIsBraceSyntaxInAField(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a produced group", `e='{a,b}'; f $e`, `1 | [{a,b}]`},
		{"beside written text", `e='{a,b}'; f x$e`, `1 | [x{a,b}]`},
		{"a produced comma", `e=a,b; f {$e}`, `1 | [{a,b}]`},
		{
			"a produced brace in a list's element", `a="x{p"; b="q}y"; set -- "$a" "$b"; f $@`,
			`2 | [x{p] [q}y]`,
		},
		{"a quoted group is not syntax either", `f "{a,b}"`, `1 | [{a,b}]`},
		{
			"nor a quoted one beside a list", `set -- 1 2; f "x{p,q}"$@y`,
			`2 | [x{p,q}1] [2y]`,
		},
		// And the written ones still are, with a list in the same word.
		{
			"the written braces still count", `set -- 1 2; f x{p,q}$@y`,
			`3 | [xp1] [xq1] [2y]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := braceFields(t, tc.src, No); out != tc.want || st != 0 {
				t.Errorf("%s = %q status %d, want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// A range is counted from the body's text however the text got there, which
// is BraceRangeEndpointsExpanded arriving on this road: the endpoints have
// already run by the time the braces are looked at, so there is nothing left
// to say about *where* in the body the expansion stood.
func TestARangeInAFieldIsCountedFromItsText(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an expanded high endpoint", `n=3; f {1..$n}`, `3 | [1] [2] [3]`},
		{"an expanded low endpoint", `n=1; f {$n..3}`, `3 | [1] [2] [3]`},
		{"the whole range produced", `e=1..3; f {$e}`, `3 | [1] [2] [3]`},
		{"a quoted endpoint is still a range", `f {1..'3'}`, `3 | [1] [2] [3]`},
		{"a comma still beats a range", `f {1..3,5}`, `2 | [1..3] [5]`},
		{
			"and a written range beside a list", `set -- 1 2; f {1..2}$@`,
			`3 | [11] [21] [2]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := braceFields(t, tc.src, No); out != tc.want || st != 0 {
				t.Errorf("%s = %q status %d, want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// The road is read off the axis rather than put to it, so a vector that has
// not answered keeps the road it had — where the fan asks at the hit, a span
// a second name is about to run again, and refuses only there.
//
// The rows that must not be refused are the ones the two readings agree
// about, and they are the common case: a word of literal text, and a word
// whose only expansion every name merely reads.
func TestAnUnansweredFanAxisKeepsItsOldRoad(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		refused   bool
	}{
		{"a group of literal text", `f {a,b}`, false},
		{"a range", `f {1..3}`, false},
		{"a parameter the names only read", `v=V; f {x,y}$v`, false},
		{"a group that is not a list", `f {a}`, false},
		{"work a second name would repeat", `f {x,y}$(echo z)`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := braceFields(t, tc.src, Unspecified)
			if tc.refused {
				if st == 0 {
					t.Fatalf("%s = %q status 0, want a refusal", tc.src, out)
				}
				return
			}
			if st != 0 {
				t.Errorf("%s = %q status %d, want no question put", tc.src, out, st)
			}
		})
	}
}

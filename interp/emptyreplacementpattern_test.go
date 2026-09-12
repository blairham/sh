// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"

	. "github.com/blairham/sh/interp"
)

// An unanchored span replacement whose *pattern* is empty, which the panel
// answers three different ways.
//
// It is a question about the pattern's text and not about empty matches in
// general: the shell that declines this pattern outright still takes an empty
// match from a pattern that has bytes in it, which is the row
// TestWhichEmptyMatchAReplacementDeclines is about. The two axes are next to
// each other and neither one implies the other, so both are asserted here
// against the same subject (#1857).
func TestWhatAnEmptyReplacementPatternMatches(t *testing.T) {
	for _, tc := range []struct {
		name, src                        string
		nothing, anEmptyValue, everyPosn string
	}{
		{
			"the global spelling written empty",
			`v=abc; printf "[%s]" "${v///X}"`,
			"[abc]", "[abc]", "[XaXbXc]",
		},
		{
			// Reached through an expansion rather than written, which is the
			// shape a script arrives at by accident: the pattern is built and
			// comes out holding nothing.
			"the pattern arriving from a value",
			`v=abc; p=; printf "[%s]" "${v//$p/X}"`,
			"[abc]", "[abc]", "[XaXbXc]",
		},
		{
			"the single spelling, which can only be reached from a value",
			`v=abc; p=; printf "[%s]" "${v/$p/X}"`,
			"[abc]", "[abc]", "[Xabc]",
		},
		{
			// The row that makes three answers out of two. One column takes
			// the pattern where there is nothing to scan and leaves every
			// other value alone.
			"an empty value",
			`v=; printf "[%s]" "${v///X}"`,
			"[]", "[X]", "[X]",
		},
		{
			// The control for the row above: an empty value is not something
			// that produces the replacement on its own.
			"an empty value under a pattern that cannot match it",
			`v=; printf "[%s]" "${v//x/X}"`,
			"[]", "[]", "[]",
		},
		{
			// Quote removal happens before the pattern is read, so a written
			// empty quotation is the same empty pattern.
			"a written empty quotation",
			`v=abc; printf "[%s]" "${v//""/X}"`,
			"[abc]", "[abc]", "[XaXbXc]",
		},
		{
			// And the common path: a pattern with a byte in it never reaches
			// the axis, so every answer gives the same value. A rule written
			// over replacements in general rather than over the empty pattern
			// would move this row.
			"a pattern that is not empty",
			`v=abc; printf "[%s]" "${v//b/X}"`,
			"[aXc]", "[aXc]", "[aXc]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, side := range []struct {
				policy EmptyReplacementPatternPolicy
				want   string
			}{
				{EmptyReplacementPatternMatchesNothing, tc.nothing},
				{EmptyReplacementPatternMatchesAnEmptyValue, tc.anEmptyValue},
				{EmptyReplacementPatternMatchesEveryPosition, tc.everyPosn},
			} {
				sem := testSemantics()
				sem.EmptyReplacementPattern = side.policy
				sem.ReplacementEmptyMatchDeclined = EmptyMatchDeclinedAtTheEnd
				out, st := runGrammar(t, tc.src, patternGrammar, withSem(sem))
				if out != side.want || st != 0 {
					t.Errorf("%v: %s = %q (status %d), want %q at 0",
						side.policy, tc.src, out, st, side.want)
				}
			}
		})
	}
}

// The anchored spellings are a row of their own and do not ask the axis.
//
// Written down because it is the boundary the axis was placed at: `${v/#/X}`
// and `${v/%/X}` have an empty pattern too, and two of the three columns that
// answer the unanchored question agree about them — so an axis asked wherever
// the pattern is empty would have refused a construct nobody disputes.
func TestAnAnchoredEmptyPatternAsksNothing(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"anchored at the front", `v=abc; printf "[%s]" "${v/#/X}"`, "[Xabc]"},
		{"anchored at the end", `v=abc; printf "[%s]" "${v/%/X}"`, "[abcX]"},
		{"anchored over an empty value", `v=; printf "[%s]" "${v/#/X}"`, "[X]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The axis left unanswered on purpose: reaching it would print a
			// refusal and end at status 2, which is what this asserts against.
			sem := testSemantics()
			out, st := runGrammar(t, tc.src, patternGrammar, withSem(sem))
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// An unanswered axis is refused by name rather than guessed at, which is what
// makes the core a core.
func TestAnUnansweredEmptyReplacementPatternIsRefused(t *testing.T) {
	sem := testSemantics()
	out, st := runGrammar(t, `v=abc; printf "[%s]" "${v///X}"`, patternGrammar, withSem(sem))
	if st != 2 || !strings.Contains(out, "empty pattern") {
		t.Fatalf("got %q (status %d), want a refusal naming the axis at status 2", out, st)
	}
}

// The same question over an array, where the substitution is applied to each
// element: the axis is read once per element and the answer is the same one.
func TestAnEmptyReplacementPatternOverAnArray(t *testing.T) {
	const src = `a=(ab c); printf "[%s]" "${a[@]///X}"`
	enable := func(d *syntax.Dialect) {
		patternGrammar(d)
		d.ArrayLiteral = true
		d.ArraySubscript = true
	}
	for _, side := range []struct {
		policy EmptyReplacementPatternPolicy
		want   string
	}{
		{EmptyReplacementPatternMatchesNothing, "[ab][c]"},
		{EmptyReplacementPatternMatchesEveryPosition, "[XaXb][Xc]"},
	} {
		sem := testSemantics()
		sem.EmptyReplacementPattern = side.policy
		sem.ReplacementEmptyMatchDeclined = EmptyMatchDeclinedAtTheEnd
		out, st := runGrammar(t, src, enable, withSem(sem))
		if out != side.want || st != 0 {
			t.Errorf("%v: %s = %q (status %d), want %q at 0", side.policy, src, out, st, side.want)
		}
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"

	. "github.com/blairham/sh/interp"
)

// alternationGrammar is the pattern grammar plus the alternation these rows
// need to write a pattern that matches either one letter or nothing.
func alternationGrammar(d *syntax.Dialect) {
	patternGrammar(d)
	d.PatternAlternation = true
}

// Which empty match a global replacement declines, once the pattern is one
// that can match empty at all.
//
// Every column agrees a match reaching the end of the value ends the scan, so
// `${v//*/X}` is one X — that rule is TestAMatchEndingAtTheValuesEndIsTheLastOne
// and it is unanimous. What splits the panel is an empty match that does not
// reach the end, and it splits two ways: one reading refuses the empty match
// waiting at the end of the value and takes the one sitting where the match
// before it ended, and the other does the opposite (#1859).
//
// The replacement is written `<>` so that every match is visible, which is the
// whole of why these rows discriminate: a replacement of one character could
// not tell "matched twice here" from "matched once".
func TestWhichEmptyMatchAReplacementDeclines(t *testing.T) {
	for _, tc := range []struct {
		name, src            string
		atTheEnd, afterMatch string
	}{
		{
			// The discriminating row. Under one reading there are two `<>`
			// between `a` and `c` — the letter match and the empty match
			// where it ended — and nothing after `c`; under the other there
			// is one, and a `<>` after `c`.
			"empty or the middle letter",
			`v=abc; printf "[%s]" "${v//(b|)/<>}"`,
			"[<>a<><>c]", "[<>a<>c<>]",
		},
		{
			// A pattern whose letter is not in the value, so every match is
			// empty and nothing is ever adjacent to anything. The two
			// readings part only at the end.
			"empty or a letter that is not there",
			`v=abc; printf "[%s]" "${v//(x|)/<>}"`,
			"[<>a<>b<>c]", "[<>a<>b<>c<>]",
		},
		{
			// The control both readings answer alike: the letter matches at
			// the last unit, so the scan ends there under either rule. A
			// change that moved this row would have broken the rule the whole
			// panel agrees on rather than chosen between the two it does not.
			"empty or the last letter",
			`v=abc; printf "[%s]" "${v//(c|)/<>}"`,
			"[<>a<>b<>]", "[<>a<>b<>]",
		},
		{
			// The letter matches first, so the adjacency is at the front.
			"empty or the first letter",
			`v=abc; printf "[%s]" "${v//(a|)/<>}"`,
			"[<><>b<>c]", "[<>b<>c<>]",
		},
		{
			// One unit, and no letter to match, so the end of the value is
			// the only position the readings part on.
			"a value of one unit",
			`v=a; printf "[%s]" "${v//(x|)/<>}"`,
			"[<>a]", "[<>a<>]",
		},
		{
			// The same value where the letter does match: it reaches the end
			// of the value, so the scan is over and both readings agree.
			"a value of one unit the letter matches",
			`v=a; printf "[%s]" "${v//(a|)/<>}"`,
			"[<>]", "[<>]",
		},
		{
			// A pattern that cannot match empty reaches neither position, so
			// the axis is never asked and both answers agree. This is the
			// common path, and a rule written over replacements in general
			// rather than over empty matches would move it.
			"a pattern that cannot match empty",
			`v=abcabc; printf "[%s]" "${v//(b|c)/<>}"`,
			"[a<><>a<><>]", "[a<><>a<><>]",
		},
		{
			// And the unanimous end-of-value rule, from the other side.
			"a star takes the value once",
			`v=abc; printf "[%s]" "${v//*/<>}"`,
			"[<>]", "[<>]",
		},
		{
			// The single-replacement spelling stops after one match, so
			// neither position is ever reached.
			"the single spelling",
			`v=abc; printf "[%s]" "${v/(b|)/<>}"`,
			"[<>abc]", "[<>abc]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, side := range []struct {
				policy EmptyMatchDeclinedPolicy
				want   string
			}{
				{EmptyMatchDeclinedAtTheEnd, tc.atTheEnd},
				{EmptyMatchDeclinedAfterAMatch, tc.afterMatch},
			} {
				sem := testSemantics()
				sem.ReplacementEmptyMatchDeclined = side.policy
				out, st := runGrammar(t, tc.src, alternationGrammar, withSem(sem))
				if out != side.want || st != 0 {
					t.Errorf("%v: %s = %q (status %d), want %q at 0",
						side.policy, tc.src, out, st, side.want)
				}
			}
		})
	}
}

// An unanswered axis is refused by name, and only where the two readings would
// have landed differently.
//
// Both halves are asserted, because the second is what says the axis is at the
// disagreement rather than on the path every substitution takes: a core with
// no answer still replaces.
func TestTheEmptyMatchAxisIsAskedOnlyAtTheDisagreement(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		refused   bool
		want      string
	}{
		{"an empty match where the last one ended", `v=abc; printf "[%s]" "${v//(b|)/<>}"`, true, ""},
		{"an empty match at the end of the value", `v=abc; printf "[%s]" "${v//(x|)/<>}"`, true, ""},
		{"a pattern that cannot match empty", `v=abc; printf "[%s]" "${v//b/<>}"`, false, "[a<>c]"},
		{"a star over the whole value", `v=abc; printf "[%s]" "${v//*/<>}"`, false, "[<>]"},
		{"one unit at a time", `v=abc; printf "[%s]" "${v//?/<>}"`, false, "[<><><>]"},
		{"a match ending at the last unit", `v=abc; printf "[%s]" "${v//(c|)/<>}"`, false, "[<>a<>b<>]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := testSemantics()
			out, st := runGrammar(t, tc.src, alternationGrammar, withSem(sem))
			if tc.refused {
				if st != 2 || !strings.Contains(out, "which empty match") {
					t.Fatalf("got %q (status %d), want a refusal naming the axis at status 2", out, st)
				}
				return
			}
			if out != tc.want || st != 0 {
				t.Fatalf("got %q (status %d), want %q at 0 — the axis was asked where the "+
					"two readings agree", out, st, tc.want)
			}
		})
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// A repetition is greedy, which is only visible through what it reports.
//
// Measured against zsh 5.9.2, 2026-09-12, and every row here is one where
// **both** divisions of the subject match. That is the whole point of the
// file: which end a star or a closure claims cannot change whether a pattern
// matches — backtracking finds a match from either end — so it is invisible
// to a `[[ ]]` and decides everything a `(#b)` group is handed.
//
// docs/spec/grammar/patterns.md carried "within an arm, as much as it can"
// before any of this worked, with three examples under it and **not one of
// them able to tell the two readings apart**: each forces the split, so a
// shell reading shortest-first answers all three correctly. This tree did
// read shortest-first, and passed them. #2513.
//
// What it cost is a real configuration rather than a corner: a theme file of
// `key = value` lines, read with the idiom every ini parser in shell uses,
// gave back every value with a leading blank — `[[:blank:]]#` matched none of
// the space and the `(*)` after it took the space along with the value.
func TestARepetitionTakesAsMuchAsItCan(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a closure takes every repetition, not none",
			`[[ xx93 == (#b)x#(*) ]] && echo "[${match[1]}]"`,
			"[93]",
		},
		{
			"and one that must repeat still takes every one",
			`[[ xx93 == (#b)x##(*) ]] && echo "[${match[1]}]"`,
			"[93]",
		},
		{
			"a star takes the lot and leaves the group empty",
			`[[ abc93 == (#b)*(*) ]] && echo "[${match[1]}]"`,
			"[]",
		},
		{
			"a repeated group reports its last repetition, not its first",
			`[[ xxy == (#b)(x)#(*) ]] && echo "[${match[1]}][${match[2]}]"`,
			"[x][y]",
		},
		{
			"a counted closure counts up to its ceiling",
			`[[ xxxxy == (#b)x(#c2,4)(*) ]] && echo "[${match[1]}]"`,
			"[y]",
		},
		{
			// The line that found it, in the POSIX spelling of a negated
			// bracket: this harness runs a Core dialect and `[^…]` is the
			// bash/zsh spelling, so the real parser's `[^[:blank:]=]` is
			// `[![:blank:]=]` here. The closure and the groups are the same,
			// which is what this row is about.
			"the line that found it",
			`[[ 'command = 93' == (#b)[[:blank:]]#([![:blank:]=]##)[[:blank:]]#[=][[:blank:]]#(*) ]] &&` +
				` echo "[${match[1]}][${match[2]}]"`,
			"[command][93]",
		},
		{
			"the bounds follow the text the group was given",
			`[[ xx93 == (#b)x#(*) ]] && echo "[${mbegin[1]}][${mend[1]}]"`,
			"[3][4]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runReportingFlags(t, tc.src)
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// TestGreedDoesNotDecideWhetherAPatternMatches is the other half, and it is
// what makes the change above safe to make in a matcher this much rides on.
//
// Reading a repetition from the other end reorders the attempts and must
// reach the same verdict, because the matcher backtracks either way. These
// are the rows that would catch a reordering that lost a match rather than
// merely reporting a different split — the failure that would not show up in
// a capture at all.
func TestGreedDoesNotDecideWhetherAPatternMatches(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// A closure that has to stop early so the rest can match.
		{`[[ aab == a#ab ]] && echo hit || echo miss`, "hit"},
		{`[[ aaab == a##ab ]] && echo hit || echo miss`, "hit"},
		// A star that has to give text back.
		{`[[ abcabc == *abc ]] && echo hit || echo miss`, "hit"},
		{`[[ aaa == *a ]] && echo hit || echo miss`, "hit"},
		// And the ones that genuinely do not match, so a greedier read
		// cannot be passing them by accident.
		{`[[ abab == ab# ]] && echo hit || echo miss`, "miss"},
		{`[[ a == ab## ]] && echo hit || echo miss`, "miss"},
		{`[[ aaaa == a(#c2,3) ]] && echo hit || echo miss`, "miss"},
	} {
		out, _ := runExtendedOperators(t, tc.src, true, nil)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

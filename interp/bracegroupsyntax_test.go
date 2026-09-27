// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// braceGroupDir builds a directory where **either** alternative of `{z,y}`
// can match, and where the reading each row is *not* asserting has a file it
// would have reached.
//
// Both halves are the point, and the second is the one that nearly shipped a
// wrong rule. `{z}?(a)` matches nothing in a directory with no `{z}` in it,
// so a row asking whether a brace that is no list frees the group reads
// `[{z}?(a)]` under **both** answers and looks like a key it is not — it was
// written up as "the brace has to have expanded" off exactly that row.
// `{z}` and `{z}a` are what make it falsifiable, `a{z}` does the same for the
// row about what stands in front of the brace, and `z@a` and `z*a` do it for
// the two openers that stay text: a marked opener in front of a live group
// would reach them.
func braceGroupDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{
		"za", "ya", "zb", "yb", "zq", "z@a", "z*a", "az", "ay", "1a", "2a",
		"{z}", "{z}a", "a{z}",
	} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// A brace that expanded frees the group syntax in the text an expansion
// produced behind it, in one column. See
// Semantics.BraceFreesProducedGroupSyntax for the panel.
func TestABraceFreeingAProducedGroup(t *testing.T) {
	dir := braceGroupDir(t)
	brace := func(t *testing.T, src string, a Answer) string {
		t.Helper()
		out, st := runGrammar(t, emptyAltCounter+src+"\n", func(d *syntax.Dialect) {
			// The construct the rows are about: a group is a group only
			// where the grammar has one, so the flag is named here rather
			// than a shell that happens to carry it.
			d.ExtendedPattern = true
		}, func(r *Runner) {
			r.Dir = dir
			s := *r.Semantics
			s.BraceExpansion = Yes
			s.BraceFanExpandsEachNameOnItsOwn = No
			s.BraceScanReadsProducedText = Yes
			s.BraceBodyReadAfterExpansion = Yes
			s.BraceOutputRereadAsText = No
			s.BraceRescanEntersFailedGroup = No
			s.BraceEmptyAlternativeIsAField = Yes
			s.BraceStopsFieldSplitting = Yes
			s.BraceRangeEndpointsExpanded = Yes
			s.BraceMakesAProducedStarOrBracketText = Yes
			s.GlobExpansionResults = Yes
			s.ExpansionResultSuppliesGroupSyntax = No
			s.BraceFreesProducedGroupSyntax = a
			r.Semantics = &s
		})
		if st != 0 {
			t.Fatalf("%s: status %d", src, st)
		}
		return strings.TrimSpace(out)
	}
	for _, tc := range []struct{ name, src, frees, text string }{
		// The control: with no brace in the word the same value is four
		// ordinary characters under both readings, which is the recorded
		// answer for this column and what makes the rows below the brace's
		// doing rather than the value's.
		{
			"no brace in the word", `g='?(a)'; f z$g`,
			`1 | [z?(a)]`, `1 | [z?(a)]`,
		},

		{
			"a produced group behind a list", `g='?(a)'; f {z,y}$g`,
			`2 | [za] [ya]`, `2 | [z?(a)] [y?(a)]`,
		},
		{
			"one opened with a plus", `g='+(a)'; f {z,y}$g`,
			`2 | [za] [ya]`, `2 | [z+(a)] [y+(a)]`,
		},
		{
			"one opened with a bang", `g='!(q)'; f {z,y}$g`,
			`6 | [z*a] [z@a] [za] [zb] [ya] [yb]`, `2 | [z!(q)] [y!(q)]`,
		},
		{
			"a produced bar in a group the script wrote", `g='a|b'; f {z,y}@($g)`,
			`4 | [za] [zb] [ya] [yb]`, `2 | [z@(a|b)] [y@(a|b)]`,
		},
		{
			"out of a command substitution", `f {z,y}$(printf '%s' '?(a)')`,
			`2 | [za] [ya]`, `2 | [z?(a)] [y?(a)]`,
		},

		// **The two openers that stay text**, and the directory is what makes
		// them falsifiable: `z*a` and `z@a` are the files a marked opener in
		// front of a live group would reach, and `za` is the one a live
		// group would. Neither reading reaches any of them, because a group
		// is introduced by one of `? * + @ !` and a bare `(…)` is text — so
		// an opener that is itself marked takes its parentheses with it.
		{
			"a group a produced star opens", `g='*(a)'; f {z,y}$g`,
			`2 | [z*(a)] [y*(a)]`, `2 | [z*(a)] [y*(a)]`,
		},
		{
			"and one a produced at-sign opens", `g='@(a)'; f {z,y}$g`,
			`2 | [z@(a)] [y@(a)]`, `2 | [z@(a)] [y@(a)]`,
		},

		// The keys are the two neighboring rules': the character rather
		// than a list, the written unquoted brace, and the rest of the word.
		{
			"a brace that is no list does it just the same", `g='?(a)'; f {z}$g`,
			`2 | [{z}] [{z}a]`, `1 | [{z}?(a)]`,
		},
		{
			"nor a quoted one", `g='?(a)'; f "{z}"$g`,
			`1 | [{z}?(a)]`, `1 | [{z}?(a)]`,
		},
		{
			"nor an escaped one", `g='?(a)'; f \{z\}$g`,
			`1 | [{z}?(a)]`, `1 | [{z}?(a)]`,
		},
		{
			"nor a produced one, although it expands", `b='{'; g='?(a)'; f ${b}z,y}$g`,
			`2 | [z?(a)] [y?(a)]`, `2 | [z?(a)] [y?(a)]`,
		},
		{
			"and what stands in front of the brace keeps its text",
			`g='?(a)'; f $g{z}`, `1 | [?(a){z}]`, `1 | [?(a){z}]`,
		},

		// A range and an empty alternative are lists like any other.
		{
			"a range frees it too", `g='?(a)'; f {1..2}$g`,
			`2 | [1a] [2a]`, `2 | [1?(a)] [2?(a)]`,
		},
		{
			"and so does an empty alternative", `g='?(a)'; f {z,}$g`,
			`2 | [za] [?(a)]`, `2 | [z?(a)] [?(a)]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := brace(t, tc.src, Yes); got != tc.frees {
				t.Errorf("frees: %s = %q, want %q", tc.src, got, tc.frees)
			}
			if got := brace(t, tc.src, No); got != tc.text {
				t.Errorf("text: %s = %q, want %q", tc.src, got, tc.text)
			}
		})
	}
}

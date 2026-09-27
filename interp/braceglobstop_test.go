// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// braceGlobDir builds a directory for the rows below, and every name in it is
// one some row's pattern can actually hit.
//
// That is the point of the helper rather than a convenience. A glob row whose
// directory holds nothing the pattern could match answers the same under both
// readings — the word passes through unchanged — so it shows a pattern
// *passing through* rather than a pattern never tried, and a table of such
// rows reads as a measurement while measuring nothing.
//
// `zaa` and `zab` are what a value with text in front of its metacharacter
// needs (`za*`, `za?`), and `{z}q` is what the rows about a brace that is no
// list need, since `{z}*` and `{z}?` have to have something to match.
func braceGlobDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{"za", "zb", "zq", "zaa", "zab", "{z}q"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// A written brace takes the match away from a `*` or a `[` an expansion
// produced behind it, in one column. See
// Semantics.BraceMakesAProducedStarOrBracketText for the panel.
//
// Through the field counter rather than `echo` for the reason every row in
// this area does: `echo` joins its arguments with a blank, so `[z*] [y*]` and
// the single field `z* y*` print the same.
func TestABraceTakingTheMatchFromAProducedPattern(t *testing.T) {
	dir := braceGlobDir(t)
	brace := func(t *testing.T, src string, a Answer) string {
		t.Helper()
		out, st := run(t, emptyAltCounter+src+"\n", func(r *Runner) {
			r.Dir = dir
			s := *r.Semantics
			s.BraceExpansion = Yes
			s.BraceFanExpandsEachNameOnItsOwn = No
			s.BraceOutputRereadAsText = No
			s.BraceRescanEntersFailedGroup = No
			s.BraceBodyReadAfterExpansion = No
			s.BraceEmptyAlternativeIsAField = No
			s.BraceStopsFieldSplitting = No
			s.GlobExpansionResults = Yes
			s.BraceMakesAProducedStarOrBracketText = a
			r.Semantics = &s
		})
		if st != 0 {
			t.Fatalf("%s: status %d", src, st)
		}
		return strings.TrimSpace(out)
	}
	for _, tc := range []struct{ name, src, takes, matches string }{
		// The two controls, and they are why the rest of the table is
		// readable: the rows below say a pattern did not match, which is
		// also what a shell that never tried says, so something here has to
		// be seen matching. The first is a produced `*` with no brace in the
		// word and the second a written one with a brace in front of it, and
		// each matches under both readings.
		{
			"no brace in the word", `g='*'; f z$g`,
			`5 | [za] [zaa] [zab] [zb] [zq]`, `5 | [za] [zaa] [zab] [zb] [zq]`,
		},
		{
			"a written star behind the brace", `f {z,y}*`,
			`6 | [za] [zaa] [zab] [zb] [zq] [y*]`, `6 | [za] [zaa] [zab] [zb] [zq] [y*]`,
		},

		{
			"a produced star behind the brace", `g='*'; f {z,y}$g`,
			`2 | [z*] [y*]`, `6 | [za] [zaa] [zab] [zb] [zq] [y*]`,
		},
		{
			"a produced bracket behind the brace", `g='[ab]'; f {z,y}$g`,
			`2 | [z[ab]] [y[ab]]`, `3 | [za] [zb] [y[ab]]`,
		},
		{
			"text in front of the produced star", `g='a*'; f {z,y}$g`,
			`2 | [za*] [ya*]`, `4 | [za] [zaa] [zab] [ya*]`,
		},

		// **A produced `?` keeps its match**, which is what says this is a
		// set of characters and not the rest of the word becoming text: a
		// rule stated the second way predicts `[z?] [y?]` here and is wrong.
		// Both rows answer the same under both readings.
		{
			"a produced question mark keeps its match", `g='?'; f {z,y}$g`,
			`4 | [za] [zb] [zq] [y?]`, `4 | [za] [zb] [zq] [y?]`,
		},
		{
			"and keeps it behind text of its own", `g='a?'; f {z,y}$g`,
			`3 | [zaa] [zab] [ya?]`, `3 | [zaa] [zab] [ya?]`,
		},

		// The character and not a group: `{z}` is no list and takes the
		// match just the same.
		{"a brace that is not a list at all", `g='*'; f {z}$g`, `1 | [{z}*]`, `1 | [{z}q]`},

		// The three that say it is the *written unquoted* brace and nothing
		// else, each answering the same under both readings.
		{"a quoted brace takes nothing", `g='*'; f "{z}"$g`, `1 | [{z}q]`, `1 | [{z}q]`},
		{"nor an escaped one", `g='*'; f \{z\}$g`, `1 | [{z}q]`, `1 | [{z}q]`},
		{
			"nor a produced one", `b='{'; g='*'; f ${b}z}$g`,
			`1 | [{z}q]`, `1 | [{z}q]`,
		},

		// And it is the *rest of the word*: what stands in front of the
		// brace still matches.
		{
			"what stands in front still matches", `g='z*'; f $g{a,b}`,
			`4 | [za] [zaa] [zab] [zb]`, `4 | [za] [zaa] [zab] [zb]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := brace(t, tc.src, Yes); got != tc.takes {
				t.Errorf("takes: %s = %q, want %q", tc.src, got, tc.takes)
			}
			if got := brace(t, tc.src, No); got != tc.matches {
				t.Errorf("matches: %s = %q, want %q", tc.src, got, tc.matches)
			}
		})
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A written brace ends field splitting for the rest of the word, in one
// column. See Semantics.BraceStopsFieldSplitting for the panel.
//
// The rows go through the field counter rather than `echo` for the reason
// every row in this area does: `echo` joins its arguments with a blank, so
// `[xpa] [b]` and `[xpa b]` print the same.
func TestABraceEndingFieldSplitting(t *testing.T) {
	stop := func(t *testing.T, src string, a Answer) (string, int) {
		t.Helper()
		out, st := run(t, emptyAltCounter+"IFS=:\n"+src+"\n", func(r *Runner) {
			s := *r.Semantics
			s.BraceExpansion = Yes
			s.BraceFanExpandsEachNameOnItsOwn = No
			s.BraceOutputRereadAsText = No
			s.BraceRescanEntersFailedGroup = No
			s.BraceBodyReadAfterExpansion = No
			s.BraceEmptyAlternativeIsAField = No
			s.BraceStopsFieldSplitting = a
			r.Semantics = &s
		})
		return strings.TrimSpace(out), st
	}
	for _, tc := range []struct{ name, src, stops, splits string }{
		// The control: with no brace in the word the two readings answer
		// the same, so nothing about the splitting itself is at stake.
		{"no brace in the word", `v=a:b; f x$v`, `2 | [xa] [b]`, `2 | [xa] [b]`},

		{
			"a group in front of the value", `v=a:b; f x{p,q}$v`,
			`2 | [xpa:b] [xqa:b]`, `3 | [xpa] [xqa] [b]`,
		},
		{
			"a brace that is not a list at all", `v=a:b; f x{p}$v`,
			`1 | [x{p}a:b]`, `2 | [x{p}a] [b]`,
		},
		{
			"what stands in front of it still splits", `v=a:b; w=c:d; f $v{p,q}$w`,
			`3 | [a] [bpc:d] [bqc:d]`, `4 | [a] [bpc] [bqc] [d]`,
		},

		// The three that say it is the *written unquoted* brace and nothing
		// else: each answers the same under both readings.
		{"a quoted brace stops nothing", `v=a:b; f "x{"$v`, `2 | [x{a] [b]`, `2 | [x{a] [b]`},
		{"nor a produced one", `v=a:b; b='{'; f x$b$v`, `2 | [x{a] [b]`, `2 | [x{a] [b]`},
		{"nor an escaped one", `v=a:b; f x\{p\}$v`, `2 | [x{p}a] [b]`, `2 | [x{p}a] [b]`},

		// A list behind the brace still makes fields of its own, which is
		// what says this is field *splitting* and not the fields.
		{
			"a list behind the brace", `set -- 1 2; f x{p,q}$@y`,
			`3 | [xp1] [xq1] [2y]`, `3 | [xp1] [xq1] [2y]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := stop(t, tc.src, Yes); out != tc.stops || st != 0 {
				t.Errorf("stops: %s = %q status %d, want %q", tc.src, out, st, tc.stops)
			}
			if out, st := stop(t, tc.src, No); out != tc.splits || st != 0 {
				t.Errorf("splits: %s = %q status %d, want %q", tc.src, out, st, tc.splits)
			}
		})
	}
}

// And the body of a group is reached by the same rule rather than by one of
// its own: a body stands behind a brace like anything else in the word.
func TestAGroupsBodyUnderTheSplittingRule(t *testing.T) {
	body := func(t *testing.T, a Answer) (string, int) {
		t.Helper()
		out, st := run(t, emptyAltCounter+`e='a b,c'; f {$e}`+"\n", func(r *Runner) {
			s := *r.Semantics
			s.BraceExpansion = Yes
			s.BraceFanExpandsEachNameOnItsOwn = No
			s.BraceOutputRereadAsText = No
			s.BraceRescanEntersFailedGroup = No
			s.BraceEmptyAlternativeIsAField = No
			s.BraceBodyReadAfterExpansion = Yes
			s.BraceStopsFieldSplitting = a
			r.Semantics = &s
		})
		return strings.TrimSpace(out), st
	}
	if out, st := body(t, Yes); out != `2 | [a b] [c]` || st != 0 {
		t.Errorf("stops: got %q status %d, want %q", out, st, `2 | [a b] [c]`)
	}
	if out, st := body(t, No); out != `2 | [{a] [b,c}]` || st != 0 {
		t.Errorf("splits: got %q status %d, want %q", out, st, `2 | [{a] [b,c}]`)
	}
}

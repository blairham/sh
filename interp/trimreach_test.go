// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
	"testing"

	"github.com/blairham/sh/syntax"
)

// fshGroup is one option group of F-Sy-H's git chroma, the shape #7275 was
// found on: a parenthesized list of alternatives a few hundred bytes long,
// which the chroma trims with `${(M)e##\(*\)(:add|:del)}`.
const fshGroup = "(-h|--help|-a|--all|-p|--patch|-u|--update|-n|--dry-run|-v|--verbose|-f|--force|" +
	"-i|--interactive|-e|--edit|-N|--intent-to-add|--refresh|--ignore-errors|--ignore-missing|" +
	"--renormalize|--no-warn-embedded-repo|--chmod|--pathspec-from-file|--pathspec-file-nul|-A|" +
	"--no-ignore-removal|--ignore-removal|--sparse|--no-all|--ignore-other|--x-option-a|--x-option-b)"

const fshPattern = `\(*\)(:add|:del)`

// shortGroup is the same shape at a length an exhaustive comparison can
// afford: the walk with the analysis off is quadratic in it under `(S)`.
const shortGroup = "(-h|--help|-a|--all)"

// alternationTrimCases are trims whose pattern holds an alternation, which
// the edge literals refuse outright and the reach analysis reads: arms of
// different lengths, an empty arm, a closure on either side of the group, two
// groups with a closure between them, and a group nothing in the subject
// matches.
var alternationTrimCases = []struct{ pattern, subject string }{
	{fshPattern, shortGroup},
	{fshPattern, shortGroup + ":add"},
	{fshPattern, shortGroup + ":del" + shortGroup + ":add"},
	{"*(:add|:del)", "x:addy:delz"},
	{"(a|ab)", "abcbcab"},
	{"(ab|a)", "abcbcab"},
	{"*(b|bc)", "abcbc"},
	{"(b|bc)*", "abcbc"},
	{"a(b|bc)c", "abcbcabc"},
	{"(foo|bar)*(baz|qux)", "foobazbarqux"},
	{"(a|ab)(c|bc)", "abcbc"},
	{"(|a)b", "aabb"},
	{"(x|y)", "abc"},
	{"*(add|dd)", "xaddyadd"},
	{"(a|x)dd", "xaddyadd"},
}

// A trim must answer what it answered before the reach analysis pruned its
// walk: the same span, the same yes or no, and the same report, under every
// reading the analysis reads, for all four operators and both searches.
//
// The `I` flag is in it because it counts matches one per start, so a start
// the analysis wrongly skipped would move which match is the second.
func TestTrimAnswersTheSameWithTheReachAsWithout(t *testing.T) {
	run := func(o patternOpts, pattern, subject string, op syntax.ParamOp, search, reach bool) string {
		defer func(prev bool) { reachDisabled = prev }(reachDisabled)
		reachDisabled = !reach
		o.where = &matchWhere{}
		lo, hi, m, ok := spanByLength(subject, pattern, op, o, search)
		return fmt.Sprintf("%d %d %v %v %v", lo, hi, ok, m.groups, m.whole)
	}
	cases := append(append(append(endSetCases, spanCases...), edgeCases...), alternationTrimCases...)
	compared := 0
	for _, rd := range reachReadings() {
		for _, tc := range cases {
			for _, op := range trimOps {
				for _, search := range []bool{false, true} {
					for _, nth := range []int{1, 2} {
						o := rd.o
						o.nth = nth
						got := run(o, tc.pattern, tc.subject, op, search, true)
						want := run(o, tc.pattern, tc.subject, op, search, false)
						compared++
						if got != want {
							t.Errorf("%s: pattern %q on %q op %v search=%v nth=%d: with the reach %q, without %q",
								rd.name, tc.pattern, tc.subject, op, search, nth, got, want)
						}
					}
				}
			}
		}
	}
	if compared == 0 {
		t.Fatal("no trim was compared")
	}
}

// The trims #7275 is about, with the answers zsh 5.9.2 gives — measured
// 2026-11-01 with `zsh -f`, the subject held in a parameter, the plain and
// the `(M)` spelling of each — so that a walk which took the wrong end of the
// ones the analysis left standing shows up as a different string.
//
// The operators are chosen so that the longest and the shortest match differ
// on every row: a pruned walk that visited the ends in the wrong order, or
// read the end set from the wrong position, would take the other one.
func TestAnAlternationTrimTakesTheEndTheOperatorAsksFor(t *testing.T) {
	g := shortGroup + ":del" + shortGroup + ":add"
	// zsh takes the arm that was written first where the arms of a longest
	// match end in different places — `${v##*(b|bc)}` is `c` there — so the
	// rows are asked with the reading that dialect makes.
	arm := armOrder{answer: Yes, ask: func() bool { return true }}
	for _, tc := range []struct {
		pattern, subject string
		op               syntax.ParamOp
		search           bool
		left, took       string
	}{
		{fshPattern, g, syntax.ParamTrimPrefix, false, shortGroup + ":add", shortGroup + ":del"},
		{fshPattern, g, syntax.ParamTrimPrefixLong, false, "", g},
		{fshPattern, g, syntax.ParamTrimSuffix, false, shortGroup + ":del", shortGroup + ":add"},
		{fshPattern, g, syntax.ParamTrimSuffixLong, false, "", g},
		{fshPattern, fshGroup, syntax.ParamTrimPrefixLong, false, fshGroup, ""},
		{fshPattern, fshGroup + ":add", syntax.ParamTrimPrefixLong, false, "", fshGroup + ":add"},
		{"*(b|bc)", "abcbc", syntax.ParamTrimPrefix, false, "cbc", "ab"},
		{"*(b|bc)", "abcbc", syntax.ParamTrimPrefixLong, false, "c", "abcb"},
		{"*(b|bc)", "abcbc", syntax.ParamTrimSuffix, false, "abc", "bc"},
		{"*(b|bc)", "abcbc", syntax.ParamTrimSuffixLong, false, "", "abcbc"},
		{"(b|bc)*", "abcbc", syntax.ParamTrimPrefix, true, "acbc", "b"},
		{"(b|bc)*", "abcbc", syntax.ParamTrimPrefixLong, true, "a", "bcbc"},
		{"(b|bc)*", "abcbc", syntax.ParamTrimSuffix, true, "abcc", "b"},
		{"(b|bc)*", "abcbc", syntax.ParamTrimSuffixLong, true, "abc", "bc"},
		{"*(add|dd)", "xaddyadd", syntax.ParamTrimPrefix, false, "yadd", "xadd"},
		{"*(add|dd)", "xaddyadd", syntax.ParamTrimPrefixLong, false, "", "xaddyadd"},
		{"(a|x)dd", "xaddyadd", syntax.ParamTrimPrefix, true, "xyadd", "add"},
		{"(a|x)dd", "xaddyadd", syntax.ParamTrimSuffix, true, "xaddy", "add"},
	} {
		// The zsh reading: a bare group, and a `*` in front of one that is a
		// closure of its own rather than the ksh quantifier on it.
		o := spanOptsForTest()
		o.quantified = false
		o.where = &matchWhere{}
		left, _ := trim(tc.subject, tc.pattern, tc.op, o, arm, tc.search)
		o.where = &matchWhere{}
		took, _ := matched(tc.subject, tc.pattern, tc.op, o, arm, tc.search)
		if left != tc.left || took != tc.took {
			t.Errorf("pattern %q on %q op %v search=%v: left %q and took %q, want %q and %q",
				tc.pattern, tc.subject, tc.op, tc.search, left, took, tc.left, tc.took)
		}
	}
}

// **The point of the change, counted rather than timed**: F-Sy-H's trim asks
// the matcher about one piece of a group that has no `:add` or `:del` after
// it, and about two at most of a group that has, whichever operator and
// search it is spelled with. The first question is the one every trim asks
// before the analysis is built, and its miss is what builds it; the second
// is the match.
//
// It used to ask about every prefix — 359 of them on this group, each a full
// walk of the `*` — because the edge literals refuse a pattern holding a `|`
// and nothing else pruned the walk (#7275). The control runs the same trims
// with the analysis off and requires them to ask about hundreds, which is
// what says the count can see a walk that was not pruned.
func TestAnAlternationTrimAsksOnlyWhereAMatchCanEnd(t *testing.T) {
	defer func() { spanAsked = nil }()
	asked := 0
	spanAsked = func(int, int) { asked++ }
	count := func(subject string, op syntax.ParamOp, search, reach bool) int {
		defer func(prev bool) { reachDisabled = prev }(reachDisabled)
		reachDisabled = !reach
		asked = 0
		o := spanOptsForTest()
		o.where = &matchWhere{}
		spanByLength(subject, fshPattern, op, o, search)
		return asked
	}
	for _, tc := range []struct {
		op     syntax.ParamOp
		search bool
	}{
		{syntax.ParamTrimPrefix, false},
		{syntax.ParamTrimPrefixLong, false},
		{syntax.ParamTrimPrefix, true},
		{syntax.ParamTrimPrefixLong, true},
		{syntax.ParamTrimSuffix, true},
		{syntax.ParamTrimSuffixLong, true},
	} {
		if n := count(fshGroup, tc.op, tc.search, true); n != 1 {
			t.Errorf("op %v search=%v: the trim asked the matcher about %d pieces of a group nothing can match, "+
				"want 1", tc.op, tc.search, n)
		}
		if n := count(fshGroup+":add", tc.op, tc.search, true); n < 1 || n > 2 {
			t.Errorf("op %v search=%v: the trim asked the matcher about %d pieces of a group with one match, "+
				"want 1 or 2", tc.op, tc.search, n)
		}
		if n := count(fshGroup, tc.op, tc.search, false); n < len(fshGroup) {
			t.Errorf("op %v search=%v: with the analysis off the trim asked about %d pieces, want every prefix "+
				"at least, so the count above cannot see an unpruned walk", tc.op, tc.search, n)
		}
	}
}

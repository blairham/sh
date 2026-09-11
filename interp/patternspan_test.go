// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// spanCases are patterns and the subjects they are tried against. The
// patterns are the ones that matter — the literals a prompt theme
// substitutes, the constructs the bound must refuse to reason about, and
// the edges between them — and the subjects are chosen so that most spans
// of them fail and a few succeed.
var spanCases = []struct{ pattern, subject string }{
	// The two that cost 6.27 of 6.33 seconds, and their shapes.
	{" %{\b", "a %{\b b %{\b {x}%{\b "},
	{" \b", "a \b b \b \b\b "},
	{"abc", "xabcabcyabc"},
	{"", ""},
	{"", "abc"},
	// An escape in front of a metacharacter is an ordinary character.
	{`\*`, "a*b*c"},
	{`\[`, "a[b[c"},
	{`a\*b`, "za*bz a*b"},
	// Bytes the analysis must read as plain, because a prompt writes them.
	{"%", "a%b%%c"},
	{"{x}", "a{x}b{x}"},
	{"}", "}}a}"},
	// And every construct it must refuse to bound.
	{"*", "abc"},
	{"a*c", "abcadc"},
	{"?", "abc"},
	{"a?c", "abcadc"},
	{"[ab]", "abcab"},
	{"[a-c]x", "axbxzx"},
	{"(a|bb)", "a bb abb"},
	{"a#", "aaab"},
	{"a##", "aaab"},
	{"^a", "abc"},
	{"a~b", "ab a"},
	{"<1-9>", "a5b12c"},
	{"(#i)AB", "ab AB Ab"},
	// A trailing backslash, and an escape the dialect may not let through.
	{`a\`, `a\b`},
	{`\a`, `a \a`},
	// Multi-byte, where a pattern byte is still one subject byte.
	{"é", "aéb"},
	{"héllo", "xhélloy"},
}

// spanOptsForTest is the widest reading, so that every construct above is
// live and the analysis is asked the hardest version of each question.
func spanOptsForTest() patternOpts {
	return patternOpts{extended: true, numericRange: true, group: true, quantified: true}
}

// **The bound may only ever exclude a span that could not have matched.**
//
// This is the whole safety property, and it is asserted directly rather
// than through the substitutions that rely on it: for every span of every
// subject, if the matcher says the pattern fills it, the bound must not
// have said it could not. A bound that is too wide costs the attempts it
// failed to skip and changes no answer; one that is too narrow silently
// drops a replacement, and nothing else in the suite would say so.
//
// Every span rather than a chosen few, because the failure this guards
// against is precisely the span nobody thought to write down.
func TestTheSpanBoundNeverExcludesASpanThatMatches(t *testing.T) {
	for _, tc := range spanCases {
		t.Run(tc.pattern+"|"+tc.subject, func(t *testing.T) {
			o := spanOptsForTest()
			lo, hi, bounded := patternSpanBytes(tc.pattern, o)
			if !bounded {
				return
			}
			for i := 0; i <= len(tc.subject); i++ {
				for j := i; j <= len(tc.subject); j++ {
					o.where = &matchWhere{}
					ok, _ := matchPatternIn(tc.pattern, tc.subject[i:j], tc.subject, i, o)
					if ok && !spanCouldMatch(j-i, lo, hi, bounded) {
						t.Fatalf("pattern %q fills %q (%d bytes) but the bound says %d..%d",
							tc.pattern, tc.subject[i:j], j-i, lo, hi)
					}
				}
			}
		})
	}
}

// And the bound has to actually bite, or the test above passes for an
// analysis that gives up on everything. The literals are the rows that
// carry the performance claim, so they are named: each must come back
// bounded, and at exactly its own length, since every byte of them is
// ordinary.
func TestTheSpanBoundIsTightOnTheLiteralsThatCostTheTime(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		want    int
	}{
		{" %{\b", 4},
		{" \b", 2},
		{"abc", 3},
		{"", 0},
		{"%", 1},
		{"{x}", 3},
		{`\*`, 1},
		{`a\*b`, 3},
		{"é", 2},
		{"héllo", 6},
	} {
		lo, hi, bounded := patternSpanBytes(tc.pattern, spanOptsForTest())
		if !bounded || lo != tc.want || hi != tc.want {
			t.Errorf("patternSpanBytes(%q) = %d..%d bounded=%v, want exactly %d",
				tc.pattern, lo, hi, bounded, tc.want)
		}
	}
}

// Every construct the analysis must refuse, named one at a time. A list
// that only checked a couple would pass for a version that had quietly
// started reasoning past one of the others.
func TestTheSpanBoundGivesUpOnEveryConstructItCannotRead(t *testing.T) {
	for _, p := range []string{
		"*", "a*c", "?", "a?c", "[ab]", "a[b-c]", "(a|b)", "a#", "a##",
		"^a", "a~b", "<1-9>", "(#i)ab", "a)", "a|b", "a>b", "a@b", "a+b", "a!b",
		`a\`,
	} {
		if _, _, bounded := patternSpanBytes(p, spanOptsForTest()); bounded {
			t.Errorf("patternSpanBytes(%q) claims a bound; it must give up", p)
		}
	}
}

// The substitution itself, end to end, over the same cases: the bound is
// wired into three loops and a mistake in any of them shows up as a
// different string. Compared against the same run with the bound forced
// off, which is the only reference that cannot drift from the code.
func TestSubstitutionAnswersTheSameWithTheBoundAsWithout(t *testing.T) {
	for _, tc := range spanCases {
		for _, anchor := range []byte{0, '#', '%'} {
			for _, all := range []bool{false, true} {
				got := replaceForTest(tc.pattern, tc.subject, anchor, all, true)
				want := replaceForTest(tc.pattern, tc.subject, anchor, all, false)
				if got != want {
					t.Errorf("pattern %q on %q anchor %q all=%v: bounded %q, unbounded %q",
						tc.pattern, tc.subject, anchor, all, got, want)
				}
			}
		}
	}
}

// replaceForTest runs one substitution, optionally with the bound
// disabled, so the two can be compared. Disabling is a rewrite of the
// pattern's analysis rather than a flag on the code under test:
// spanBoundDisabled is read by patternSpanBytes and by nothing else.
func replaceForTest(pattern, subject string, anchor byte, all, bound bool) string {
	defer func(prev bool) { spanBoundDisabled = prev }(spanBoundDisabled)
	spanBoundDisabled = !bound
	o := spanOptsForTest()
	o.where = &matchWhere{}
	e := replaceExprForTest(anchor, all)
	return replace(subject, pattern, e, o, func(matchReport) string { return "<>" })
}

func TestSpanStoppersCoversEveryMetacharacterTheMatcherKnows(t *testing.T) {
	// The two sets the matcher itself uses. If either grows, this fails
	// until the stopper list grows with it — which is the coupling that
	// keeps the analysis honest as the grammar moves.
	for _, set := range []string{patternMeta, extendedPatternMeta} {
		for i := 0; i < len(set); i++ {
			if set[i] == '\\' {
				continue // handled as an escape rather than stopped at
			}
			if strings.IndexByte(spanStoppers, set[i]) < 0 {
				t.Errorf("%q is a metacharacter to the matcher and not a span stopper", set[i])
			}
		}
	}
}

// replaceExprForTest is the one node `replace` reads: the anchor and
// whether every match is replaced.
func replaceExprForTest(anchor byte, all bool) *syntax.ParamExpr {
	return &syntax.ParamExpr{Anchor: anchor, All: all}
}

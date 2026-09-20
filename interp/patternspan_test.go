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
	// The two that consume exactly one unit, which the bound does read.
	{"?", "abc"},
	{"a?c", "abcadc"},
	{"??", "abc"},
	{"[ab]", "abcab"},
	{"[a-c]x", "axbxzx"},
	{"[!a]", "abcab"},
	{"[]a]", "a]b"},
	{"[0-9]", "a1b22c"},
	{"x[a-c]?y", "xacy xbzy xy"},
	// A bracket holding a sub-expression, which it must not: the readings
	// of an unclosed or unknown one can make the whole text ordinary.
	{"[[:digit:]]", "a1b22c"},
	{"[[:nope:]]", "a1b]c"},
	{"[[:digit:]", "a1b]c"},
	{"[[.a.]]", "a[.b"},
	{"[a[b]", "a[b] ab"},
	// A bracket nothing closes, whose text is ordinary.
	{"[ab", "a[abz"},
	// And a closure over either of them, which has no upper bound at all.
	{"?#", "abc"},
	{"[ab]#", "abcab"},
	// And every construct it must refuse to bound.
	{"*", "abc"},
	{"a*c", "abcadc"},
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
	// And multi-byte under a unit: this is where the upper bound stops
	// being the lower one, because a `?` here takes two bytes and not one.
	{"?", "aéb"},
	{"[éx]", "aéb xé"},
	{"a?b", "aéb ab aééb"},
}

// spanOptsForTest is the widest reading, so that every construct above is
// live and the analysis is asked the hardest version of each question.
func spanOptsForTest() patternOpts {
	return patternOpts{extended: true, numericRange: true, group: true, quantified: true}
}

// spanReadings is the same widest reading twice, over the two answers to
// what a unit *is*. They are not interchangeable to this analysis: a `?`
// consumes one byte where the matcher counts bytes and up to four where it
// counts characters, so a bound proved sound over one says nothing about
// the other — and the byte reading is the one that would pass for an upper
// bound of 1 that silently loses every multi-byte match.
func spanReadings() []struct {
	name string
	o    patternOpts
} {
	bytes, chars := spanOptsForTest(), spanOptsForTest()
	chars.chars = true
	return []struct {
		name string
		o    patternOpts
	}{{"bytes", bytes}, {"characters", chars}}
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
	for _, rd := range spanReadings() {
		t.Run(rd.name, func(t *testing.T) {
			for _, tc := range spanCases {
				t.Run(tc.pattern+"|"+tc.subject, func(t *testing.T) {
					o := rd.o
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
		})
	}
}

// The bound on the two constructs that consume one unit, stated exactly,
// because "it is bounded" is not the claim — an upper bound of the rest of
// the subject is what made the substitution quadratic, and a bound that
// widened to it would still pass the safety property above.
//
// Both readings, because the two differ: a unit is one byte where the
// matcher counts bytes and one to four where it counts characters.
func TestTheSpanBoundReadsOneUnitAsOneUnit(t *testing.T) {
	for _, tc := range []struct {
		pattern            string
		lo, byteHi, charHi int
	}{
		{"?", 1, 1, 4},
		{"??", 2, 2, 8},
		{"[ab]", 1, 1, 4},
		{"[!a]", 1, 1, 4},
		{"[]a]", 1, 1, 4},
		{"[0-9]", 1, 1, 4},
		{"a?c", 3, 3, 6},
		{"x[a-c]?y", 4, 4, 10},
	} {
		for _, rd := range spanReadings() {
			want := tc.byteHi
			if rd.o.chars {
				want = tc.charHi
			}
			lo, hi, bounded := patternSpanBytes(tc.pattern, rd.o)
			if !bounded || lo != tc.lo || hi != want {
				t.Errorf("patternSpanBytes(%q) counting %s = %d..%d bounded=%v, want %d..%d",
					tc.pattern, rd.name, lo, hi, bounded, tc.lo, want)
			}
		}
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
		"*", "a*c", "(a|b)", "a#", "a##",
		"^a", "a~b", "<1-9>", "(#i)ab", "a)", "a|b", "a>b", "a@b", "a+b", "a!b",
		`a\`,
		// A bracket that is not one, or that holds a sub-expression: every
		// shape some reading makes ordinary text of.
		"[ab", "[", "[]", "[[:digit:]]", "[[:nope:]]", "[[:digit:]", "[[.a.]]",
		"[[=a=]]", "[a[b]",
		// And a closure over a unit, which is zero or more of them.
		"?#", "[ab]#",
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
	for _, rd := range spanReadings() {
		for _, tc := range spanCases {
			for _, anchor := range []byte{0, '#', '%'} {
				for _, all := range []bool{false, true} {
					got := replaceForTest(rd.o, tc.pattern, tc.subject, anchor, all, true)
					want := replaceForTest(rd.o, tc.pattern, tc.subject, anchor, all, false)
					if got != want {
						t.Errorf("pattern %q on %q anchor %q all=%v counting %s: bounded %q, unbounded %q",
							tc.pattern, tc.subject, anchor, all, rd.name, got, want)
					}
				}
			}
		}
	}
}

// replaceForTest runs one substitution, optionally with the bound
// disabled, so the two can be compared. Disabling is a rewrite of the
// pattern's analysis rather than a flag on the code under test:
// spanBoundDisabled is read by patternSpanBytes and by nothing else.
func replaceForTest(o patternOpts, pattern, subject string, anchor byte, all, bound bool) string {
	defer func(prev bool) { spanBoundDisabled = prev }(spanBoundDisabled)
	spanBoundDisabled = !bound
	o.where = &matchWhere{}
	e := replaceExprForTest(anchor, all)
	return replace(subject, pattern, e, o, armOrder{},
		func() EmptyMatchDeclinedPolicy { return EmptyMatchDeclinedAtTheEnd },
		func(matchReport, string) string { return "<>" })
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

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// trimOps is every operator edgeByLength orders candidates for, so a
// difference that shows up in only one of the four readings is still caught.
var trimOps = []syntax.ParamOp{
	syntax.ParamTrimPrefix, syntax.ParamTrimPrefixLong,
	syntax.ParamTrimSuffix, syntax.ParamTrimSuffixLong,
}

// **The edge literals may only ever exclude a piece that could not have
// matched.**
//
// The same safety property the span bound has, asserted the same way and for
// the same reason: too *short* a literal costs the attempts it failed to skip
// and changes no answer, while one claiming bytes the pattern does not
// require would drop a trim's match and nothing else here would say so.
//
// Every span of every subject rather than a chosen few, because the failure
// this guards against is the piece nobody thought to write down.
func TestTheEdgeLiteralsNeverExcludeAPieceThatMatches(t *testing.T) {
	for _, tc := range append(spanCases, edgeCases...) {
		t.Run(tc.pattern+"|"+tc.subject, func(t *testing.T) {
			o := spanOptsForTest()
			head, tail, known := patternEdgeLiterals(tc.pattern, o)
			if !known {
				return
			}
			for i := 0; i <= len(tc.subject); i++ {
				for j := i; j <= len(tc.subject); j++ {
					o.where = &matchWhere{}
					ok, _ := matchPatternIn(tc.pattern, tc.subject[i:j], tc.subject, i, o)
					if ok && !edgeLiteralsFit(tc.subject[i:j], head, tail, known) {
						t.Fatalf("pattern %q matches %q, but the edges %q..%q exclude it",
							tc.pattern, tc.subject[i:j], head, tail)
					}
				}
			}
		})
	}
}

// edgeCases are the shapes a trim meets that the span bound had no reason to
// carry: a pattern that is all `*` and one literal run, which is the shape
// that cost the time, and the alternations and negations that must be refused.
var edgeCases = []struct{ pattern, subject string }{
	{"*abc", "xxabcyyabc"},
	{"*abc*", "xxabcyyabc"},
	{"abc*", "abcxabc"},
	{"*a*b", "zazbzazb"},
	{"*\x1ekey\x1f", "a\x1ekey\x1fb\x1ekey\x1fc"},
	{"a*|b", "ab ba"},
	{"*(a|b)c", "xac xbc"},
	{"*a~*b", "za zb"},
	{"*^a", "za zb"},
	{`*a\*`, "za* zb*"},
	{"a#b", "b ab aab"},
	{"*a#b", "zb zab"},
	{"xa#b", "xb xab"},
}

// And the analysis has to actually bite, or the property above holds for one
// that gives up on everything. These are the rows that carry the performance
// claim: what the pattern requires at each end, named exactly.
func TestTheEdgeLiteralsAreTheCharactersEveryMatchCarries(t *testing.T) {
	for _, tc := range []struct {
		pattern    string
		head, tail string
	}{
		// The shape measured on powerlevel10k's instant-prompt cache: no head
		// at all, and a tail that excludes all but one of 13.5 thousand
		// candidate prefixes.
		{"*\x1ekey\x1f", "", "\x1ekey\x1f"},
		{"*abc", "", "abc"},
		{"abc*", "abc", ""},
		{"a*b", "a", "b"},
		{"a*", "a", ""},
		{"*", "", ""},
		// One literal run is both edges at once, and each is checked on its
		// own: a two-byte piece satisfies neither.
		{"abc", "abc", "abc"},
		{"", "", ""},
		// An escaped metacharacter is an ordinary character in the run, and
		// the run is the character rather than the two bytes that wrote it.
		{`*a\*`, "", "a*"},
		{`\[x*`, "[x", ""},
		// A bracket, a quantifier and a range all stop a run without
		// refusing the pattern: what follows the last of them is still
		// required.
		{"[ab]xy", "", "xy"},
		{"?xy", "", "xy"},
		{"<1-9>xy", "", "xy"},
		// `#` quantifies what precedes it, so the run before one is given up:
		// `a#bc` matches `bc`, which does not begin with `a`.
		{"a#bc", "", "bc"},
		{"x[a]#bc", "x", "bc"},
	} {
		head, tail, known := patternEdgeLiterals(tc.pattern, spanOptsForTest())
		if !known || head != tc.head || tail != tc.tail {
			t.Errorf("patternEdgeLiterals(%q) = %q..%q known=%v, want %q..%q",
				tc.pattern, head, tail, known, tc.head, tc.tail)
		}
	}
}

// Every construct that can let a match end somewhere the pattern's last
// characters do not, named one at a time. A list that only checked a couple
// would pass for a version that had quietly started reasoning past one of
// the others.
func TestTheEdgeLiteralsGiveUpOnWhatCouldBypassThem(t *testing.T) {
	for _, p := range []string{
		"a|b", "*a|b", "(a|b)", "^a", "a~b", "*a~b", "(#i)ab", "(#b)(a)b",
		`a\`,
	} {
		if _, _, known := patternEdgeLiterals(p, spanOptsForTest()); known {
			t.Errorf("patternEdgeLiterals(%q) claims edges; it must give up", p)
		}
	}
	// Case folding is a run-time answer rather than a pattern one, and it
	// makes a literal's bytes the wrong question: `(#i)` is refused above by
	// its spelling, and these two by their options.
	folding := spanOptsForTest()
	folding.fold = true
	if _, _, known := patternEdgeLiterals("*abc", folding); known {
		t.Error("patternEdgeLiterals claims edges under fold; it must give up")
	}
	lit := spanOptsForTest()
	lit.litFold = caseEither
	if _, _, known := patternEdgeLiterals("*abc", lit); known {
		t.Error("patternEdgeLiterals claims edges under litFold; it must give up")
	}
}

// The trim itself, end to end, over the same cases and all four operators:
// the edges are wired into one loop with two arms, and a mistake in either
// shows up as a different string. Compared against the same run with the
// analysis forced off, which is the only reference that cannot drift from
// the code.
func TestTrimAnswersTheSameWithTheEdgeLiteralsAsWithout(t *testing.T) {
	for _, tc := range append(spanCases, edgeCases...) {
		for _, op := range trimOps {
			got := trimForTest(tc.pattern, tc.subject, op, true)
			want := trimForTest(tc.pattern, tc.subject, op, false)
			if got != want {
				t.Errorf("pattern %q on %q op %v: with edges %q, without %q",
					tc.pattern, tc.subject, op, got, want)
			}
		}
	}
}

// trimForTest runs one trim, optionally with the analysis disabled, so the
// two can be compared. Disabling is a rewrite of the pattern's analysis
// rather than a flag on the code under test: spanBoundDisabled is read by
// patternEdgeLiterals and patternSpanBytes and by nothing else.
func trimForTest(pattern, subject string, op syntax.ParamOp, edges bool) string {
	defer func(prev bool) { spanBoundDisabled = prev }(spanBoundDisabled)
	spanBoundDisabled = !edges
	o := spanOptsForTest()
	o.where = &matchWhere{}
	out, _ := trim(subject, pattern, op, o, armOrder{})
	return out
}

// The expansion this was found in, at the size it was found at.
//
// `${content##*$rs$key$us}` against powerlevel10k's instant-prompt cache was
// 2139ms here and 0.125ms in zsh 5.9.2 — every prefix of a 13.5KB subject
// handed to the matcher, where all but one of them end in bytes the pattern
// does not. The answer is what is asserted; the time it takes is the point,
// and a run that has lost the analysis takes seconds rather than failing.
func TestTheLongestPrefixTrimOnARealPromptCache(t *testing.T) {
	const key = "\x1e/home/user:0:%\x1f"
	var b strings.Builder
	b.WriteString("head")
	b.WriteString(key)
	for b.Len() < 13*1024 {
		b.WriteString("\x1e/home/other:0:%\x1fpadding-that-never-matches;")
	}
	subject := b.String()
	want := subject[len("head")+len(key):]

	o := spanOptsForTest()
	o.where = &matchWhere{}
	got, _ := trim(subject, "*"+key, syntax.ParamTrimPrefixLong, o, armOrder{})
	if got != want {
		t.Errorf("trim left %d bytes, want %d", len(got), len(want))
	}
}

func BenchmarkLongestPrefixTrimOnAPromptCache(b *testing.B) {
	const key = "\x1e/home/user:0:%\x1f"
	var sb strings.Builder
	sb.WriteString("head")
	sb.WriteString(key)
	for sb.Len() < 13*1024 {
		sb.WriteString("\x1e/home/other:0:%\x1fpadding-that-never-matches;")
	}
	subject := sb.String()
	for b.Loop() {
		o := spanOptsForTest()
		o.where = &matchWhere{}
		trim(subject, "*"+key, syntax.ParamTrimPrefixLong, o, armOrder{})
	}
}

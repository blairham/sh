// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"slices"
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// reachTestOpts is the options a dialect with alternation, closures, ranges
// and the `(#…)` flags builds — the one whose startup #1575 was found in.
func reachTestOpts(pattern string) patternOpts {
	o := patternOpts{
		caret:        true,
		bracket:      BracketLiteral,
		group:        true,
		quantified:   true,
		numericRange: true,
		extended:     true,
		escapes:      `-=!*?[]()|^~#<>\`,
	}
	o.where = &matchWhere{plan: planCapturesFor(pattern, o)}
	return o
}

// reachCases are patterns whose search the bound narrows, paired with
// subjects on both sides of the answer.
//
// The point of each is a *split* the arms or the item cannot reach: `(:|=)`
// covers one unit and `(ab|c)` two, so every longer split of the subject is a
// question the search used to ask anyway. The unbounded shapes are here for
// the other direction — the bound must not narrow a pattern whose reach its
// own text does not say, or these answers change.
var reachCases = []struct {
	pattern  string
	subjects []string
}{
	// The two shapes the startup in #1575 spun on, in miniature.
	{" ##", []string{"", " ", "   ", "  x", "x  ", "\\\\\\\\"}},
	{"(:|=)", []string{"", ":", "=", "::", ":x", "x"}},
	// An item and arms that reach further than one unit.
	{"(ab|c)", []string{"ab", "c", "abc", "a", ""}},
	{"(ab|c)##", []string{"ab", "cc", "abcab", "abx", ""}},
	{"[a-z]##", []string{"abc", "", "ab1", "1ab"}},
	{`\a##`, []string{"aaa", "a", "", "ab"}},
	{"?##", []string{"abc", "", "a"}},
	// Bounded, and the bound is not one unit: the whole pattern's length.
	{"(a|bb|ccc)x", []string{"ax", "bbx", "cccx", "ccccx", "x"}},
	{"@(ab|c)d", []string{"abd", "cd", "d", "abcd"}},
	{"?(ab)c", []string{"abc", "c", "ababc"}},
	// Unbounded by their own text, so the bound must leave them alone. The
	// arm is where this has to be checked and not the whole pattern: the
	// reach that bounds a search is an *arm's* or an *item's*, so a closure
	// standing alone is never asked, and a bound that read one as bounded
	// would go unnoticed without these.
	// Each matching subject reaches further than the arm's own text is
	// long, which is what a bound reading one of these as bounded would cut
	// short. A subject of three `a`s proves nothing against `a##`, because
	// three is what len("a##") allows anyway.
	{"(a##|b)c", []string{"ac", "aaaaaaaac", "bc", "c"}},
	{"(+(a)|b)c", []string{"ac", "aaaaaaaac", "bc", "c"}},
	{"(a<->|b)c", []string{"a7c", "a1234567890c", "bc", "c"}},
	{"((ab)##|c)d", []string{"abd", "ababababababd", "cd", "d"}},
	{"(a*|b)c", []string{"ac", "aaaaaaaac", "bc", "c"}},
	{"(^ab|c)d", []string{"xyzwvud", "abd", "cd", "d"}},
	{"(a|b)##c", []string{"abababc", "c", "ac"}},
	{"+(ab)c", []string{"ababc", "abc", "c"}},
	{"!(ab)c", []string{"xc", "abc", "c"}},
	{"^ab", []string{"ax", "ab", ""}},
	{"<1-100>x", []string{"7x", "100x", "101x", "x"}},
	// A bracket holds the very characters the scan looks for, and they are
	// ordinary members inside one.
	{"[#^~*]##", []string{"##", "^~*", "", "#a"}},
	// An escaped operator is a literal, and the escape is stepped over.
	{`\#\#x`, []string{"##x", "#x", "x"}},
	// `(#i)` folds and consumes nothing, but carries a `#`.
	{"(#i)(AB|c)d", []string{"abd", "ABD", "cd", "d"}},
}

// TestTheReachBoundChangesNoAnswer runs every case with the search stopping
// at the pattern's reach and with it running to the end of the subject, and
// requires the two to agree on the match and on what a `(#b)` reported.
//
// Both halves matter. A bound that is too tight answers "no" to a match that
// exists, which is the failure this repository is built to catch, and a bound
// that silently changes which split won would report a different `$match`
// while still answering yes.
func TestTheReachBoundChangesNoAnswer(t *testing.T) {
	was := patternReachBounds
	t.Cleanup(func() { patternReachBounds = was })
	for _, c := range reachCases {
		for _, subject := range c.subjects {
			patternReachBounds = false
			o := reachTestOpts(c.pattern)
			wantOK, wantReport := matchPatternIn(c.pattern, subject, subject, 0, o)

			patternReachBounds = true
			o = reachTestOpts(c.pattern)
			gotOK, gotReport := matchPatternIn(c.pattern, subject, subject, 0, o)

			if gotOK != wantOK {
				t.Errorf("%q against %q = %v bounded, %v unbounded", c.pattern, subject, gotOK, wantOK)
				continue
			}
			if gotReport.whole != wantReport.whole {
				t.Errorf("%q against %q reported whole %v bounded, %v unbounded",
					c.pattern, subject, gotReport.whole, wantReport.whole)
			}
			if !slices.Equal(gotReport.groups, wantReport.groups) {
				t.Errorf("%q against %q reported %v bounded, %v unbounded",
					c.pattern, subject, gotReport.groups, wantReport.groups)
			}
		}
	}
}

// TestPatternReach pins what each shape's own text says about how far it can
// go, because the bound is only sound while this is right and the test above
// would pass a reach that is merely *too large*.
func TestPatternReach(t *testing.T) {
	for _, c := range []struct {
		pattern string
		want    int
	}{
		{" ##", unboundedReach}, // the closure is what makes it so
		{" ", 1},
		{"(:|=)", 5},
		{"(ab|c)", 6},
		{"[a-z]", 5},
		{`\a`, 2},
		{"?", 1},
		{"@(ab|c)d", 8},
		{"?(ab)c", 6},
		{"a*b", unboundedReach},
		{"+(ab)", unboundedReach},
		{"!(ab)", unboundedReach},
		{"^a", unboundedReach},
		{"a~b", unboundedReach},
		{"<1-9>", unboundedReach},
		{"[#^~*]", 6}, // every operator, ordinary inside a bracket
		{`\#\^x`, 5},  // and escaped outside one
		{"(#i)ab", unboundedReach},
	} {
		o := reachTestOpts(c.pattern)
		if got := patternReach(c.pattern, &o); got != c.want {
			t.Errorf("patternReach(%q) = %d, want %d", c.pattern, got, c.want)
		}
	}
}

// TestATrimOverALongValueAsksALinearNumberOfQuestions is #1575 itself, and it
// is written as a *count* rather than as a clock.
//
// A wall-clock assertion is the obvious way to pin a hang and the wrong one
// here: this suite runs several packages at once on a loaded machine, so a
// timing bound either has to be so loose that the regression fits inside it
// or it manufactures failures of its own. The number of questions the matcher
// puts to itself does not move with the load, and it is the thing that was
// wrong — the search asked one per split of the subject, per prefix a trim
// tried, which is the square of the length.
//
// The value is a run of backslashes because that is what the startup was
// holding: `${x## ##}` at line 129 of the meta-plugin annex's before-load
// handler, over a value that had grown to 524,629 characters. Neither pattern
// can match one of them, so every trial fails and the whole search is the
// cost.
//
// The bound is generous — a small multiple of the length, where the fix makes
// it a little over one per prefix — because the point is the shape of the
// growth and not a golden count that every unrelated change would have to
// update.
//
// The length is chosen so that a regression *fails* rather than hanging the
// suite, which is the whole difficulty in pinning this bug: the value that
// found it was half a megabyte and the search over it does not finish in an
// afternoon, so a row of that size in the corpus would take the run with it.
// Ten thousand is enough that the two shapes cannot be confused — the fix
// asks about ten thousand questions and the search without it about fifty
// million, a ratio of five thousand against an allowance of eight — and small
// enough that the failing side still returns in under a second.
func TestATrimOverALongValueAsksALinearNumberOfQuestions(t *testing.T) {
	const n = 10_000
	value := strings.Repeat(`\`, n)
	for _, c := range []struct {
		name    string
		pattern string
		op      syntax.ParamOp
	}{
		{"a closure of one space", " ##", syntax.ParamTrimPrefixLong},
		{"an alternation of one character", "(:|=)", syntax.ParamTrimPrefixLong},
		{"the same from the other end", "(:|=)", syntax.ParamTrimSuffixLong},
	} {
		t.Run(c.name, func(t *testing.T) {
			o := reachTestOpts(c.pattern)
			got, _ := trim(value, c.pattern, c.op, o)
			if got != value {
				t.Fatalf("trim removed %d bytes; no run of backslashes matches %q",
					len(value)-len(got), c.pattern)
			}
			if asked := o.where.asked; asked > 8*n {
				t.Errorf("asked %d questions over %d bytes — more than a constant per prefix, "+
					"so the search is walking splits the pattern cannot reach", asked, n)
			}
		})
	}
}

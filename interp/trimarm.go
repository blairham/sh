// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A longest trim's second reading: which arm of an alternation the pattern
// takes when the arms take different lengths.
//
// `${x##pat}` is spelled "the longest match", and one shell in the panel does
// not search for one. It tries the arms of an alternation in the order they
// were written and keeps the first that lets the whole pattern match, so
// `x=abc; ${x##(a|ab)}` is `bc` there and `c` everywhere else. The arm is a
// *preference* rather than a restriction — `${x##(a|ab)c}` still empties the
// value, because the first arm leaves the `c` nothing to match and the search
// falls back to the second — and within the arm it takes as much as it can:
// `${x##(a*|ab)}` empties it too.
//
// The matcher already knows this rule on the half of it that reports: a
// written arm beats a longer one for what `(#b)` captures, which matchGroup
// states and pins. What it cannot do is say so through a yes/no answer, which
// is all trimSpan asks it — the search there drives the *candidate* order,
// longest piece first, so the arm the matcher would have preferred is decided
// before the matcher is ever consulted.
//
// So the arm order is recovered by asking a different question: resolve the
// alternations in the pattern to one arm each, in the order a written arm
// beats a later one, and ask the ordinary question of each resolved pattern.
// The first one that matches anywhere is the reading, and the length search
// inside it is the "as much as it can" half. The chosen edge is then matched
// once more with the *original* pattern, so `$match` and the `(M)` flag see
// the pattern the script wrote rather than a variant of it — and see the same
// arm, because matchGroup prefers a written arm on its own.
//
// Which reading a dialect uses is Semantics.LongestMatchTakesTheWrittenArm,
// and it is asked only where the two readings land in different places.
//
// **The prefix trim is where it is reachable without a flag, not where it
// stops.** A longest *suffix* trim pins the end of the match to the end of
// the value, so the arms have no length to disagree about and the panel
// answers alike: measured, `v=abcbc` gives `ab` for both `${v%%(bc|cbc)}`
// and `${v%%(cbc|bc)}`. Under the `(S)` flag that end comes loose and the
// same split appears — `w=abc`, `${(S)w%%(b|bc)}` is `ac` and
// `${(S)w%%(bc|b)}` is `a` — which is why writtenArmReaches asks about the
// *shape of the match* rather than naming the operator.

import (
	"sort"
	"strings"
)

// armOrder is the axis and the question that resolves it.
//
// The question is a closure and not an Answer because it is asked **only at
// the disagreement** — an unanswered axis is refused by name, and a pattern
// whose two readings agree must not be refused for having an alternation in
// it. The answer comes along beside it so that the dialect which reads the
// longest match pays nothing: there is no second search to compare against
// when No is already the answer.
//
// The zero value never searches and never asks, which is what a caller that
// is not a parameter trim passes.
type armOrder struct {
	answer Answer
	// ask is the question, or nil where r is to be asked it.
	ask func() bool
	r   *Runner
}

// reaches is whether asking is worth the preparation: a dialect that reads
// the longest match has already answered, and the zero value is what a caller
// that is not a parameter trim or substitution passes.
func (a armOrder) reaches() bool {
	return a.answer != No && (a.ask != nil || a.r != nil)
}

// asked puts the question.
func (a armOrder) asked() bool {
	if a.ask != nil {
		return a.ask()
	}
	return a.r.askArmOrder()
}

// armVariantLimit is how many resolved patterns the search will consider.
//
// A pattern with n alternations of k arms has k^n of them, and the point of
// the bound is that a pathological pattern falls back to the length reading
// rather than taking the run with it. Sixty-four is far past anything a
// script writes — three alternations of four arms — and far short of a cost
// worth measuring.
const armVariantLimit = 64

// armSearch is the written-arm reading prepared for one pattern: the arms
// resolved to one variant each, in preference order, with what each variant
// requires of a piece beside it.
//
// Prepared once and asked many times, because a **substitution** asks this
// question at every position of the subject where a trim asks it once.
// Resolving the alternations is a rewrite of the pattern text and an
// allocation per variant; doing that per position turned a linear scan into
// one that rebuilt the same four strings for every character of a prompt.
type armSearch struct {
	variants []string
	bounds   []armBound
	// where is a memo of this search's own, one per variant.
	//
	// **The matcher's memo is dropped whenever the pattern moves** — see
	// matchPatternIn — so asking about a variant through the caller's
	// matchWhere throws away everything the whole pattern's scan had learned,
	// and the next question about the whole pattern rebuilds it. Measured on
	// the prompt theme's pattern that #1383 and #1398 are about: sharing the
	// caller's memo cost the substitution 22% where a memo per variant costs
	// nothing measurable, and the matcher questions were never the expense.
	//
	// A capture plan of the variant's own goes with it. The search discards
	// the report it gets, so the plan only has to be the right *shape* for
	// the pattern being matched rather than for the one the script wrote.
	where []*matchWhere
	// reach is each unbounded variant's end analysis, made from autos the
	// first time it is asked for and nil for a variant it does not read,
	// and ends is that analysis bound to the subject last asked about. A substitution asks at every position a match was found
	// at, and without it each question was every span from the length
	// reading's edge down, per variant — two thirds of #1398's substitution
	// once the length reading itself stopped enumerating (#6356). See
	// interp/substreach.go.
	reach []*reachSet
	ends  []*reachEnds
	// autos are the variants' automata, from which reach is made on first
	// use, and o the options it is made under.
	autos []*reachAutomaton
	o     patternOpts
	// stops is the unit boundaries of the subject last asked about, which a
	// substitution asks about at every match it finds.
	stops *subjectStops
}

// subjectStops is unitStops of one subject, kept for the next question about
// the same one.
type subjectStops struct {
	value string
	stops []int
}

// armBound is what one resolved variant requires of a piece, so that a
// candidate the variant could not match is skipped rather than handed to the
// matcher.
//
// The same two analyses interp/patternspan.go already performs for the length
// reading, asked of the rewritten pattern rather than of the one the script
// wrote — which is the only pattern this search ever matches against. Without
// them the arm reading is a full scan of the subject at every position, so a
// bounded pattern that costs the length reading one question would cost this
// one thousands.
type armBound struct {
	least, most int
	bounded     bool
	head, tail  string
	edges       bool
}

// newArmSearch prepares the reading, or reports that there is none to have.
//
// The second result is false for a pattern with no alternation to resolve,
// and for one whose alternations cannot be resolved by rewriting — see
// armVariants — in which case the caller keeps the length reading it has.
func newArmSearch(pattern string, o patternOpts) (armSearch, bool) {
	// The cheap half of the question first: a pattern with no bar in it
	// anywhere has one reading, and that is nearly every pattern a script
	// writes. A bar inside a bracket expression is an ordinary member and
	// reaches the scan below, which reads it as one.
	if !strings.ContainsRune(pattern, '|') {
		return armSearch{}, false
	}
	t := armTemplateFor(pattern, &o)
	if t == nil {
		return armSearch{}, false
	}
	where := make([]*matchWhere, len(t.variants))
	for i := range t.variants {
		where[i] = &matchWhere{plan: t.plans[i]}
	}
	autos := t.autos
	if reachRefused(&o) {
		// The template's automata were read without the match state, and
		// this match carries state the analysis does not read.
		autos = nil
	}
	o.where = nil
	return armSearch{
		variants: t.variants, bounds: t.bounds, where: where,
		autos: autos, o: o,
		reach: make([]*reachSet, len(t.variants)),
		ends:  make([]*reachEnds, len(t.variants)),
		stops: &subjectStops{},
	}, true
}

// readPatternBounds is both of interp/patternspan.go's analyses of p.
func readPatternBounds(p string, o patternOpts) armBound {
	var b armBound
	b.least, b.most, b.bounded = patternSpanBytes(p, o)
	b.head, b.tail, b.edges = patternEdgeLiterals(p, o)
	return b
}

// patternBoundsOf is readPatternBounds kept per pattern and reading: a trim
// or a substitution a loop runs reads the same pattern every time round, and
// for a short one like `*/` these two scans were a sixth of the trim (#6353).
func patternBoundsOf(p string, o *patternOpts) armBound {
	if !patternFactsCached || spanBoundDisabled {
		return readPatternBounds(p, stateless(*o))
	}
	rd := readingOf(o)
	e := programs.entry(p, &rd)
	b := e.bounds.Load()
	if b == nil {
		v := readPatternBounds(p, stateless(*o))
		b = &v
		e.bounds.Store(b)
	}
	return *b
}

// armTemplate is the part of an armSearch that is a fact about the pattern
// under one reading — the variants, what each requires of a piece, and each
// one's capture plan — kept per pattern and reading so that a substitution a
// loop runs builds it once (#6380). The memos and the end analyses bound to
// a subject are what a search writes, and those are made per search.
//
// A pattern with no reading to resolve is recorded as a template with no
// variants.
type armTemplate struct {
	variants []string
	bounds   []armBound
	plans    []capturePlan
	// autos is each unbounded variant's end analysis, nil for a variant it
	// does not read.
	autos []*reachAutomaton
}

// armTemplateFor is pattern's template under o, or nil where there is no
// written-arm reading to have.
func armTemplateFor(pattern string, o *patternOpts) *armTemplate {
	// The two switches a test turns to compare readings change what the
	// template holds, so a template built under either is not kept.
	if !patternFactsCached || spanBoundDisabled || reachDisabled {
		return nonEmptyTemplate(buildArmTemplate(pattern, stateless(*o)))
	}
	rd := readingOf(o)
	e := programs.entry(pattern, &rd)
	t := e.arms.Load()
	if t == nil {
		t = buildArmTemplate(pattern, stateless(*o))
		e.arms.Store(t)
	}
	return nonEmptyTemplate(t)
}

func nonEmptyTemplate(t *armTemplate) *armTemplate {
	if len(t.variants) == 0 {
		return nil
	}
	return t
}

// buildArmTemplate resolves pattern's alternations and reads each variant.
// Nothing here reads match state: the analyses split and bound pattern text,
// and the plan numbers its groups.
func buildArmTemplate(pattern string, o patternOpts) *armTemplate {
	variants, ok := armVariants(pattern, o)
	if !ok || len(variants) < 2 {
		return &armTemplate{}
	}
	t := &armTemplate{
		variants: variants,
		bounds:   make([]armBound, len(variants)),
		plans:    make([]capturePlan, len(variants)),
		autos:    make([]*reachAutomaton, len(variants)),
	}
	for i, v := range variants {
		t.bounds[i] = readPatternBounds(v, o)
		t.plans[i] = planCapturesFor(v, o)
		if !t.bounds[i].bounded {
			t.autos[i] = reachAutomatonOf(v, o)
		}
	}
	return t
}

// endAt is where the match beginning at start stops when the arms are
// searched in the order they were written.
//
// start is 0 for the unflagged prefix trim, which is where this reading was
// first needed. Under `(S)` the match may begin anywhere, and a substitution
// asks at every position — in both cases the position is chosen before the
// arm is, measured: `v=abcbc` and `${(S)v%%(bc|cbc)}` is `abc` in both
// written orders, the arm that starts closest to the end rather than the arm
// that was written first. So the position is an argument here and never
// something this reading gets to move.
//
// # limit is the length reading's answer, and it bounds this one
//
// A variant is the pattern with one arm chosen at each alternation, so every
// piece a variant matches is a piece the **whole pattern** matches — and the
// length reading has already found the longest of those. So no arm can reach
// past it, and the search starts there rather than at the end of the value.
//
// That is what keeps this affordable at every position of a substitution
// rather than only once at a trim. The common case becomes one question: the
// first variant is asked about the length reading's own edge, and where it
// matches there the two readings agree and nothing more is asked. Without the
// bound, a prompt theme's pattern over the 82-byte message it is written for
// cost **2.3x** the substitution with the reading switched off; with it, the
// difference is in the noise.
//
// The second result is whether any arm matched there at all.
func (s armSearch) endAt(value string, o patternOpts, start, limit int) (int, bool) {
	// The same candidate order a longest match uses, because the arm decides
	// *which* match and not how much of it: within one resolved pattern the
	// longest piece still wins.
	if s.stops.stops == nil || s.stops.value != value {
		s.stops.value, s.stops.stops = value, unitStops(value, o)
	}
	stops := s.stops.stops
	from := sort.SearchInts(stops, start)
	top := sort.SearchInts(stops, limit+1) - 1
	// Where only one variant could begin here at all, the search is
	// already decided. The variants between them match every piece the
	// pattern matches — an alternation resolved each way it can be is the
	// alternation — so the length reading's own match at limit is a match of
	// some variant, and the end analysis has ruled out every variant but
	// one. That one matches at limit, nothing before it matches here, and
	// nothing past limit is a match of the pattern: the walk below would
	// return limit after asking the matcher, which on #1398's substitution
	// was a second full match at every match found (#6380).
	if s.onlyOneCanBegin(value, stops, start) {
		return limit, true
	}
	for n, v := range s.variants {
		b := s.bounds[n]
		o.where = s.where[n]
		last := top
		if b.bounded {
			last = min(last, sort.SearchInts(stops, start+b.most+1)-1)
		}
		e := s.endsFor(n, value, stops)
		if e != nil && !e.from(start) {
			continue
		}
		for k := last; k >= from; k-- {
			if b.bounded && stops[k]-start < b.least {
				break
			}
			if e != nil && !e.may(stops[k]) {
				continue
			}
			piece := value[start:stops[k]]
			if !edgeLiteralsFit(piece, b.head, b.tail, b.edges) {
				continue
			}
			if ok, _ := matchPatternIn(v, piece, value, start, o); ok {
				return stops[k], true
			}
		}
	}
	// Every arm was tried and none matched, so the pattern does not match
	// this value at all — which the caller has already established before
	// asking, since it asks only about a match it found.
	return 0, false
}

// endsFor is variant n's end analysis bound to value, or nil for a variant
// the analysis does not read.
func (s armSearch) endsFor(n int, value string, stops []int) *reachEnds {
	r := s.reach[n]
	if r == nil {
		// Made the first time the variant is asked about, which under the
		// shortcut below is often never.
		if s.autos == nil || s.autos[n] == nil {
			return nil
		}
		r = newReachSetOn(s.autos[n], s.o)
		s.reach[n] = r
	}
	e := s.ends[n]
	if e == nil || e.value != value {
		e = newReachEnds(r, value, stops)
		s.ends[n] = e
	}
	return e
}

// onlyOneCanBegin reports whether the end analysis rules out every variant
// but one at start. A variant it does not read is never ruled out.
func (s armSearch) onlyOneCanBegin(value string, stops []int, start int) bool {
	if armShortcutDisabled {
		return false
	}
	open := 0
	for n := range s.variants {
		if s.autos != nil && s.autos[n] != nil && !s.autos[n].mayBegin(value, start) {
			continue
		}
		if e := s.endsFor(n, value, stops); e == nil || e.from(start) {
			open++
			if open > 1 {
				return false
			}
		}
	}
	return open == 1
}

// armShortcutDisabled turns onlyOneCanBegin off, so that a test can run the
// search both ways and compare. Nothing outside a test writes it.
var armShortcutDisabled bool

// armVariants is the pattern with its alternations resolved to one arm each,
// ordered so that a written arm comes before a later one.
//
// The arms are substituted in place, parentheses and all: `(a|ab)c` becomes
// `(a)c` and `(ab)c` rather than `ac` and `abc`, so a flag written inside the
// group — `((#i)a|b)` — keeps the group it is scoped to.
//
// The second result is false when the pattern holds an alternation this
// rewriting cannot speak for, and the caller then keeps the length reading:
//
//   - a *quantified* group, `@(a|b)` and its four relatives, whose arm may be
//     taken more than once or not at all, so one arm chosen once is not the
//     same pattern;
//   - a group a closure repeats, `(a|b)#` and `(a|b)(#c2,3)`, for the same
//     reason;
//   - a pattern with more variants than armVariantLimit.
//
// Those are the shapes where the arm order and the length order are hardest
// to tell apart and the rarest in a script; recording the limit here is
// better than a search that reaches the wrong one of them quietly.
func armVariants(pattern string, o patternOpts) ([]string, bool) {
	var out []string
	if !expandArms(pattern, o, &out) {
		return nil, false
	}
	return out, true
}

// expandArms appends every resolution of p to out, in preference order.
func expandArms(p string, o patternOpts, out *[]string) bool {
	if len(*out) >= armVariantLimit {
		return false
	}
	// A bar standing outside every group is an alternation of the whole
	// pattern where the dialect reads one, and its arms are arms like any
	// other: measured, `L='a|ab'; x=abc; ${x##${~L}}` is `bc` in the shell
	// that answers Yes, the same reading the written group gets.
	if o.topGroup {
		if arms, _ := topAlternatives(p, o.emptyBracket, o.bareParenIsText); len(arms) > 1 {
			for _, arm := range arms {
				if !expandArms(arm, o, out) {
					return false
				}
			}
			return true
		}
	}
	start, body, rest, found, decidable := firstArmedGroup(p, o)
	if !decidable {
		return false
	}
	if !found {
		*out = append(*out, p)
		return true
	}
	head := p[:start]
	for _, arm := range alternatives(body, o.emptyBracket) {
		// The substituted group has one arm, so the next pass walks past it
		// and finds the next alternation — including one nested inside the
		// arm just chosen.
		if !expandArms(head+"("+arm+")"+rest, o, out) {
			return false
		}
	}
	return true
}

// firstArmedGroup is the first group in p whose body holds more than one arm:
// where it begins, its body, and the pattern text after it.
//
// decidable is false for an alternation that cannot be resolved by choosing
// an arm — see armVariants for which those are and why.
//
// The prepared tables are deliberately not consulted: they were built for the
// pattern the script wrote, and this walks the rewritten ones too, where an
// offset means something else. Scanning a pattern is cheap; reading a stale
// closing parenthesis out of a table is a wrong answer.
func firstArmedGroup(p string, o patternOpts) (start int, body, rest string, found, decidable bool) {
	o.where = nil
	for i := 0; i < len(p); i++ {
		switch p[i] {
		case '\\':
			i++
			continue
		case '[':
			// Past the whole bracket expression: a bar between two members
			// is not an arm, which is the rule topAlternatives states.
			if end, ok := bracketEnd(p, i, o.emptyBracket); ok {
				i = end
			}
			continue
		}
		b, quant, r, ok := splitGroup(p[i:], i, &o)
		if !ok {
			continue
		}
		if len(alternatives(b, o.emptyBracket)) < 2 {
			// A flag group, or a group of one arm. The scan carries on into
			// it rather than over it, so an alternation nested inside is
			// found on the way past.
			continue
		}
		if quant != 0 {
			return 0, "", "", false, false
		}
		if _, _, _, isClosure := closureBounds(r, &o); isClosure {
			return 0, "", "", false, false
		}
		return i, b, r, true, true
	}
	return 0, "", "", false, true
}

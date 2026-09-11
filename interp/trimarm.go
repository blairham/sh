// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// The longest prefix trim's second reading: which arm of an alternation the
// pattern takes when the arms take different lengths.
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
// is all trimEdge asks it — the search there drives the *candidate* order,
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
// Which reading a dialect uses is Semantics.LongestPrefixTrimTakesTheWrittenArm,
// and it is asked only where the two readings land in different places.

import "strings"

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
	ask    func() bool
}

// armVariantLimit is how many resolved patterns the search will consider.
//
// A pattern with n alternations of k arms has k^n of them, and the point of
// the bound is that a pathological pattern falls back to the length reading
// rather than taking the run with it. Sixty-four is far past anything a
// script writes — three alternations of four arms — and far short of a cost
// worth measuring.
const armVariantLimit = 64

// writtenArmEdge is where a longest prefix trim stops when the arms are
// searched in the order they were written.
//
// The second result is whether the question could be answered at all. It is
// false for a pattern with no alternation to resolve, and for one whose
// alternations cannot be resolved by rewriting — see armVariants — in which
// case the caller keeps the length reading it already has.
func writtenArmEdge(value, pattern string, o patternOpts) (int, bool) {
	// The cheap half of the question first: a pattern with no bar in it
	// anywhere has one reading, and that is nearly every pattern a script
	// writes. A bar inside a bracket expression is an ordinary member and
	// reaches the scan below, which reads it as one.
	if !strings.ContainsRune(pattern, '|') {
		return 0, false
	}
	variants, ok := armVariants(pattern, o)
	if !ok || len(variants) < 2 {
		return 0, false
	}
	// The same candidate order a longest prefix trim uses, because the arm
	// decides *which* match and not how much of it: within one resolved
	// pattern the longest piece still wins.
	idx := unitStops(value, o)
	for l, r := 0, len(idx)-1; l < r; l, r = l+1, r-1 {
		idx[l], idx[r] = idx[r], idx[l]
	}
	for _, v := range variants {
		for _, i := range idx {
			if ok, _ := matchPatternIn(v, value[:i], value, 0, o); ok {
				return i, true
			}
		}
	}
	// Every arm was tried and none matched, so the pattern does not match
	// this value at all — which the caller has already established before
	// asking, since it asks only about a match it found.
	return 0, false
}

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
		if arms, _ := topAlternatives(p); len(arms) > 1 {
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
	for _, arm := range alternatives(body) {
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
			if end, ok := bracketEnd(p, i); ok {
				i = end
			}
			continue
		}
		b, quant, r, ok := splitGroup(p[i:], i, &o)
		if !ok {
			continue
		}
		if len(alternatives(b)) < 2 {
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

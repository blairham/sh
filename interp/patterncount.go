// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"math"
	"strings"
)

// countedGroup is a `{n,m}(…)` peeled off the front of a pattern: a
// repetition count written in braces in front of a group.
//
// counts is what separates the two readings, and it is **not** what decides
// whether the text is read at all. See splitCountedGroup.
type countedGroup struct {
	// bound is how many times the group may repeat, read only when counts.
	bound repeatBound
	// counts says the brace held a count. Where it does not, the whole of
	// text is ordinary characters and nothing else here is set.
	counts bool
	// lead is the width of the `{…}` in front of the `(`.
	lead int
	// body is what stands between the parentheses, and rest what follows
	// the `)`. Both are read only when counts.
	body, rest string
}

// splitCountedGroup peels a `{…}(…)` off the front of a pattern.
//
// **Whether the brace holds a count decides nothing about whether this
// reads the text**, and that is the whole shape of the construct. ksh93
// suppresses the brace by what follows the `}`, so a brace whose contents
// are not a count at all is still not a list — and what it becomes instead
// is ordinary characters rather than a refusal. Measured 2026-09-27 on
// `/bin/ksh` `Version AJM 93u+ 2012-08-01`:
//
//	[[ '{z,y}(a)'   == {z,y}(a)   ]]   matches — the braces and the
//	                                   parentheses are all characters
//	[[ '{z,y}a'     == {z,y}(a)   ]]   no — so the group is not read either
//	[[ '{z,y}(a|b)' == {z,y}(a|b) ]]   matches — and the bar is a character
//	                                   too, having no group to divide
//	[[ aa           == {2,3}(a)   ]]   matches — where it is a count
//
// **What it is not is quoted**, which is the row that separates "the
// parentheses stop being syntax" from "the run goes literal", and the two
// look identical over the four rows above:
//
//	[[ '{z,y}(ab)'  == {z,y}(a?)    ]]   matches — the `?` is live
//	[[ '{z,y}(ab)'  == {z,y}(a*)    ]]   matches
//	[[ '{z,y}(abb)' == {z,y}(a+(b)) ]]   matches — and a group inside is one
//
// So the caller takes the `{…}` and the `(` behind it as characters and
// reads on from there: the `)` is an ordinary character because nothing
// opened a group for it to close, the bar is one because
// [Runner.markWrittenBars] has already escaped a written bar, and anything
// with a meaning of its own keeps it. Nothing is rewritten and no offset
// moves, which matters because the prepared tables and every capture are
// keyed on a position in this pattern.
//
// ok is false where p does not open one at all — no `{`, no `}`, or no `(`
// straight behind it — and the caller carries on with the branches it had.
func splitCountedGroup(p string, pp int, o *patternOpts) (countedGroup, bool) {
	if !o.counted || p == "" || p[0] != '{' {
		return countedGroup{}, false
	}
	end := strings.IndexByte(p, '}')
	// `{}` is not one: the lexer refuses the parenthesis behind an empty
	// brace outright — `echo A{}(a)B` is a syntax error on ksh93u+ where
	// `echo A{,}(a)B` is the word — so this side answers it the same way
	// rather than reading text the grammar would never have admitted.
	if end < 2 || end+1 >= len(p) || p[end+1] != '(' {
		return countedGroup{}, false
	}
	g := countedGroup{lead: end + 1}
	b, counts := countBound(p[1:end])
	if !counts {
		// Not a count, and then there is no group here to find a `)` for:
		// the caller spends `{…}(` as characters and reads the rest of the
		// pattern exactly as it would have.
		return g, true
	}
	close, found := closingParenAt(o, p[end+1:], pp+end+1)
	if !found {
		// A count in front of a parenthesis nothing closes. The same answer
		// splitGroup gives a group that never closes: this is not one, and
		// the text is characters.
		return countedGroup{}, false
	}
	g.counts, g.bound = true, b
	g.body = p[end+2 : end+1+close]
	g.rest = p[end+1+close+1:]
	return g, true
}

// countBound reads the count between a group's braces.
//
// The grammar is `n`, `n,m`, `n,` or `,m`, in decimal, and either side may
// be omitted: an absent lower bound is nought and an absent upper one is no
// ceiling. Measured 2026-09-27 on `/bin/ksh` `Version AJM 93u+ 2012-08-01`
// over `[[ $s == p ]]`, with s in `""`, `a`, `aa`, `aaa` and `aaaa`:
//
//	           ""     a     aa    aaa   aaaa
//	{2,3}(a)   no     no   yes   yes     no
//	{2}(a)     no     no   yes    no     no
//	{2,}(a)    no     no   yes   yes    yes
//	{,2}(a)   yes    yes   yes    no     no
//	{,}(a)    yes    yes   yes   yes    yes
//	{0,0}(a)  yes     no    no    no     no
//	{,0}(a)   yes     no    no    no     no
//	{3,2}(a)   no     no    no    no     no
//
// The last row is a count no other spelling can reach: a ceiling under the
// floor. The answer is that the group matches nothing at all rather than
// that the count is refused — and it needs **no branch of its own**, which
// is worth writing down because one was added and then taken out again after
// the mutant that removed it survived. A floor of three is spent one
// repetition at a time and a ceiling of two stops the recursion before the
// third, so the floor is never reached and the match fails where it always
// would. A guard in front of it is an equivalent mutant; the rows are kept
// and the branch is not.
//
// ok is false where the text is not a count, and that is a reading rather
// than a refusal — the caller leaves the whole construct standing as
// ordinary characters. Everything that is not two decimal runs around at
// most one comma lands here, and each of these is measured to match its own
// text and nothing else: `{z,y}`, `{a}`, `{-1,2}`, `{+2,3}`, `{2,3,4}`, and
// `{2,4294967296}` — the last because a bound too large to hold is not a
// bound, which is the answer a numeric range already gives for the same
// reason (see splitNumericRange).
func countBound(text string) (repeatBound, bool) {
	lo, hi, comma := strings.Cut(text, ",")
	if !comma {
		n, ok := countSide(lo)
		if !ok || n < 0 {
			return repeatBound{}, false
		}
		return repeatBound{lo: n, hi: n}, true
	}
	l, ok := countSide(lo)
	if !ok {
		return repeatBound{}, false
	}
	h, ok := countSide(hi)
	if !ok {
		return repeatBound{}, false
	}
	if l < 0 {
		// An omitted lower bound is nought: `{,2}(a)` takes `a` and `aa`.
		l = 0
	}
	return repeatBound{lo: l, hi: h}, true
}

// countSide reads one side of a count: a run of decimal digits, or none at
// all, which is -1 — no ceiling for the upper side, and nought for the lower.
//
// A sign is not part of it. `{-1,2}(a)` and `{+2,3}(a)` each match their own
// text on ksh93u+ and nothing else, so a signed side is not a count.
func countSide(s string) (int, bool) {
	if s == "" {
		return -1, true
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
		if n > maxRepeatCount {
			// A count too large to be a count. Refusing to read it leaves
			// the text standing, which is the same answer the shell gives.
			return 0, false
		}
	}
	return n, true
}

// maxRepeatCount is the largest repetition count read as one, and the value
// is **measured rather than chosen**: the reference reads the side into a
// signed 32-bit integer and anything wider is not a count at all. The edge
// is exact, 2026-09-27 on `/bin/ksh` `Version AJM 93u+ 2012-08-01`:
//
//	[[ aa == {2,2147483647}(a) ]]   matches
//	[[ aa == {2,2147483648}(a) ]]   does not — one more, and the text is
//	                                ordinary characters
//	[[ aa == {2,4294967296}(a) ]]   does not
//
// A ceiling this large costs nothing to admit: matchGroupTimes counts it
// down only as repetitions *match*, and every repetition consumes at least
// one byte of the subject, so the recursion is bounded by the subject and
// never by the count.
const maxRepeatCount = math.MaxInt32

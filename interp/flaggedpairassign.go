// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// A pair on the left of an assignment whose ends carry flag groups:
// `a[(r)q,(r)r]=v`. Each end names a position exactly as the same end does on
// the read side — see Runner.rangeEnd — and the value replaces the span
// between them, which is what an unflagged pair already does. Measured
// 2026-10-02 on zsh 5.9.2 (`-f`, `LC_ALL=C`), with `a=(p q r s)` and
// `s=hello`:
//
//	a[(r)q,(r)r]=(x y)    p x y s      the span the two searches name
//	a[(R)nf,(r)nf]=(x y)  x y          both misses: before the first, after the last
//	a[(r)q,3]=Z           p Z s        one end flagged, one written
//	a[1,(r)q]=Z           Z r s
//	a[2,(i)r]=Z           p Z s        an index letter is a position too
//	a[(r)r,(r)q]=Z        p q Z r s    a span that ends before it starts inserts
//	a[(r)q,(r)nf]=Z       p Z          a miss past the end takes the rest
//	a[(r)q,(r)r]=()       p s          and no words delete the span
//	s[(r)l,(r)o]=Z        heZ          over a string, a span of characters
//	a[(i)q,3]=Z           invalid subscript, and the line ends
//	a[(r)q,(r)r]+=Z       p q rZ s     `+=` reads no range: the second end
//	a[(r)q,(r)r]+=(X)     p q r X s    is the one element it appends to
//
// Before this the group at the front was read as the whole subscript's, so
// `a[(r)q,3]=Z` searched for the text `q,3`, missed and appended, and a pair
// flagged only at its second end was read as arithmetic and refused (#5152).

// flaggedAssignSpan resolves the two ends of such a pair to positions. A
// refusal has already been reported, and ends the line, where ok is false.
func (r *Runner) flaggedAssignSpan(a *syntax.Assign) (from, to int, ok bool) {
	e := &syntax.ParamExpr{Name: a.Name, IndexRange: a.IndexRange, Src: a.Name + "[" + a.IndexText + "]"}
	elems, scalar, _ := r.subscriptTarget(e)
	src := subscriptSource{name: a.Name, elems: elems, scalar: scalar}
	if from, ok = r.rangeEnd(e, a.IndexRange.Lo, src, true); ok {
		to, ok = r.rangeEnd(e, a.IndexRange.Hi, src, false)
	}
	if !ok {
		// Refused on the left of `=`, where nothing was written and the line
		// must not go on to report the status of whatever came before it —
		// the rule a single flagged subscript's refusal follows (#1536).
		r.expandErr = false
		r.fatalQuiet()
		return 0, 0, false
	}
	return from, to, true
}

// assignFlaggedSpan is the scalar spelling: one word replaces the span, or is
// appended to the element the second end names.
func (r *Runner) assignFlaggedSpan(a *syntax.Assign) {
	from, to, ok := r.flaggedAssignSpan(a)
	if !ok {
		return
	}
	value := r.assignValue(a)
	if a.Append {
		r.appendArrayElem(a.Name, to, a.IndexText, value)
		return
	}
	if r.subscriptSplicesCharacters(a.Name) {
		if r.spanIsBelowTheFirstElement(from, to) {
			r.fatal("%s\n", Wording(r.diag().BadArraySubscript,
				"%[1]s[%[2]s]: bad array subscript", a.Name, a.IndexText))
			return
		}
		r.spliceCharacterSpan(a.Name, from, to, value, false)
		return
	}
	elems, _ := r.arrayElemsOfTheName(a.Name)
	r.spliceElementSpan(a.Name, a.IndexText, elems, from, to, []string{value})
}

// spliceElemLiteralOverAFlaggedSpan is the literal spelling: the words replace
// the span, or go in after the element the second end names under `+=`.
func (r *Runner) spliceElemLiteralOverAFlaggedSpan(a *syntax.Assign) {
	if !r.spliceTargetIsAnArray(a) {
		return
	}
	from, to, ok := r.flaggedAssignSpan(a)
	if !ok {
		return
	}
	build := func() ([]string, bool) { return r.literalWords(a.Name, a.Elems) }
	if a.Append {
		// The second end is one subscript here, and the splice that takes
		// one subscript is the one to hand it to.
		r.spliceWordsIntoElement(a, itoa(to), build)
		return
	}
	words, ok := build()
	if !ok {
		return
	}
	elems, _ := r.arrayElemsOfTheName(a.Name)
	r.spliceElementSpan(a.Name, a.IndexText, elems, from, to, words)
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// A keyed literal's trace lines, written as its elements are stored.
//
// The one column that traces `a=([k]=v …)` as the element writes it performs
// writes each of those lines after that element's store, not all of them in
// front of the literal: measured 2026-10-15 on ksh93u+ 2012-08-01, script
// files under `set -x` with `typeset -A a` first,
//
//	a=(["a b"]=1 [""]=3 [x]=y)   `+ a['a b']=1`, `warning: adding empty
//	                             subscript`, `+ a['']=3`, `+ a[x]=y`
//	a=([k]=1 [*]=s)              `+ a[k]=1`, then `*: invalid subscript in
//	                             assignment`, and nothing more
//
// so a warning the store makes sits beside the element it is about, and a
// refusal partway through leaves the lines of the elements in front of it and
// none behind. This wrote every line first and then stored, putting the
// warning after the last line and writing a line for the refused element
// (#6683, #6686).
//
// The lines themselves are still built in one place, before the store, by
// traceLiteralAsElementWrites — only when each is written moves.

// elementLinesFollowTheirStores reports whether a's trace is a keyed
// literal's element writes, and where it is, holds the lines on e for the
// store to write as it goes.
func (r *Runner) elementLinesFollowTheirStores(a *syntax.Assign, e *expandedAssign) bool {
	if e == nil || a.Index != nil || a.Members != nil || !r.tracing() {
		return false
	}
	lines, ok := r.traceLiteralAsElementWrites(a, e, *r.diag())
	if !ok {
		return false
	}
	e.elementLines = lines
	return true
}

// traceStoredElementLine writes the held line of the element the keyed store
// has just stored. parsed is the list that store is placing, and nothing is
// written unless it is the held literal's own — a literal nested inside an
// element is another list and has no lines here.
func (r *Runner) traceStoredElementLine(parsed []literalElem) {
	e := r.expanded
	if e == nil || e.elementLinesWritten >= len(e.elementLines) || !sameElemList(e.elems, parsed) {
		return
	}
	r.writeHeldElementLine(e)
}

// traceUnstoredElementLines writes the held lines no store wrote, after the
// assignment: a literal whose keys turned out to be positions is placed whole,
// and its lines follow the whole placement. Nothing is written where the
// assignment gave up, because the lines left are those of elements a refusal
// stopped short of.
func (r *Runner) traceUnstoredElementLines(e *expandedAssign) {
	if e == nil || r.assignmentGaveUp() {
		return
	}
	for e.elementLinesWritten < len(e.elementLines) {
		r.writeHeldElementLine(e)
	}
}

func (r *Runner) writeHeldElementLine(e *expandedAssign) {
	d := r.diag()
	r.awaitTraceTurn()
	r.traceLine(e.elementLines[e.elementLinesWritten], *d)
	r.releaseTraceTurn()
	e.elementLinesWritten++
}

// sameElemList reports whether two expanded element lists are the same slice.
func sameElemList(a, b []literalElem) bool {
	if len(a) != len(b) {
		return false
	}
	return len(a) == 0 || &a[0] == &b[0]
}

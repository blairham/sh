// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// What the **colon** of `${a[@]:-word}` tests when the parameter is a whole
// list rather than one value. Three readings, and no two of them agree on
// every row — see Semantics.WholeArrayColonTest, which holds the panel.
//
// The colon-less form is a different question and already has one:
// Semantics.EmptyArrayIsSet asks whether a list with no elements is a set
// parameter, which is the `-`/`+` test with no colon on it.

// wholeListColonFires answers the colon test for a whole list, and reports
// whether it answered at all.
//
// **Asked only where the readings differ**, which is the rule this file is
// built on. All three agree that a list of ordinary values is not null and
// that a list with no elements is, so a probe on either shape cannot tell
// them apart and putting the axis in front of it would make every
// `"${@:-default}"` in every script read an axis that decides nothing. What
// parts them is a list holding an empty element: the join reading is null
// only when the whole join is empty, the first-element reading looks no
// further than element one, and the counting reading calls any element at all
// a value.
//
// The `[*]` spelling is where the counting reading stops being one: measured,
// `f=(""); "${f[*]:-x}"` is `[x]` in the column that counts and in the column
// that joins alike, where `"${f[@]:-x}"` is `[]` in the first and `[x]` in the
// second. So the count is read for the fields spelling and the join for the
// joined one, which is what `count` carries below rather than a second axis.
func (r *Runner) wholeListColonFires(e *syntax.ParamExpr, value string, set bool) (bool, bool) {
	if !set {
		// Unset is null under every reading, and the caller's own test says
		// so without needing the elements.
		return false, false
	}
	elems, fields, ok := r.wholeListElements(e)
	if !ok {
		return false, false
	}
	join := value == ""
	first := len(elems) == 0 || elems[0] == ""
	count := len(elems) == 0
	if !fields {
		count = join
	}
	if join == first && join == count {
		// The three readings agree, so there is nothing to ask. This is the
		// common shape by a long way: every list of ordinary values, and
		// every empty one.
		return join, true
	}
	switch r.sem().WholeArrayColonTest {
	case WholeArrayColonTestReadsTheJoinedValue:
		return join, true
	case WholeArrayColonTestReadsTheFirstElement:
		return first, true
	case WholeArrayColonTestCountsTheElementsUnderAt:
		return count, true
	}
	r.diagf("%s\n", r.unanswered("what the colon of `${a[@]:-word}` tests on a whole list"))
	r.status, r.unspecified = 2, true
	return join, true
}

// wholeListElements is the elements behind a parameter that names a whole
// list, and whether the spelling is the one that keeps its fields.
//
// Both sources, because the panel splits the same way for each: measured
// 2026-09-18, `set -- "" c; "${@:-x}"` is `[x]` in ksh93u+ and two fields in
// bash 5.3.20, zsh 5.9.2 and dash 0.5.12, and `set -- ""; "${@:-x}"` is `[]`
// in zsh 5.9.2 alone — the same three readings the array rows give. The
// positional list is the commoner of the two by far, so answering only the
// array would have left the axis unreachable from most scripts.
func (r *Runner) wholeListElements(e *syntax.ParamExpr) ([]string, bool, bool) {
	if e.Inner != nil || e.Length || e.Indirect {
		return nil, false, false
	}
	if e.Index == nil {
		if e.Name == "@" || e.Name == "*" {
			return r.params(), e.Name == "@", true
		}
		return nil, false, false
	}
	if !r.wholeArrayIndex(e) {
		return nil, false, false
	}
	elems, ok := r.arraySubscript(e)
	return elems, r.atArrayIndex(e), ok
}

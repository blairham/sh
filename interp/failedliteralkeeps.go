// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// keepWhatAFailedLiteralStored is what is left of a name after an array
// literal whose first element names its subscript fails to expand partway
// through, in the column that builds such a literal as it goes. See
// Semantics.FailedSubscriptedLiteralKeepsWhatItStored for the measurement.
//
// parsed is the elements before the failed one — literalElems trims the one
// that failed. Every other literal, and every other column, stores nothing on
// a failure and the name keeps what it held; this returns without asking in
// all of those cases, so the axis is put only to a subscripted literal that
// has already failed (#6608).
//
// indexedLetter says the literal is the operand of a declaration carrying the
// indexed-array letter, which keeps the pairs as an indexed array rather than
// as a table: `typeset -a a=([2]=x [0]=$((1/0)))` is `typeset -a a=([2]=x)`.
func (r *Runner) keepWhatAFailedLiteralStored(name string, elems []*syntax.ArrayElem, parsed []literalElem, appendTo, indexedLetter bool) {
	if name == "" || r.unspecified || len(elems) == 0 || elems[0].Word == nil ||
		!syntax.SubscriptedElement(elems[0].Word) {
		// Not a subscripted literal, or the list was refused by an axis no
		// dialect answered and nothing about it is evidence.
		return
	}
	if !r.ask(r.sem().FailedSubscriptedLiteralKeepsWhatItStored,
		"a failed subscripted array literal keeping what it had stored") {
		return
	}
	table := r.assocDeclared(name)
	if appendTo {
		// An append onto a table has added the pairs before the failure; one
		// onto an indexed array has added nothing.
		if table {
			r.assignAssocElems(name, parsed, true, false)
		}
		return
	}
	if indexedLetter && !table {
		// The declaration's letter decides the kind, so the pairs land by
		// their evaluated subscripts in a fresh indexed array.
		if built, ok := r.literalInto(name, NewArray(0), 0, parsed, true); ok {
			r.storeArray(name, built)
		}
		return
	}
	// The name is started over as a table holding what was placed, whatever
	// it was before — an indexed array, a scalar, a name with a type letter.
	r.clearTypeAttributes(name)
	r.markAssoc(name)
	r.assignAssocElems(name, parsed, false, false)
}

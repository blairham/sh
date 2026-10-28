// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// `read -A` into an indexed array's element: the two shells with the letter
// land the words in different places. See
// Semantics.ReadArrayIntoAnElementRefillsTheArray, where the rows are (#6869).

// readArrayIntoAnIndexedElement is `read -A 'b[i]'` where `b` is not a table
// the dialect nests into, and reports whether it answered the operand. A
// refusal that ends the script has said so through r.ctl; one the builtin
// stops at with the line going on leaves its status in gaveUp.
//
// The subscript is read and refused as the scalar store through the same
// operand reads and refuses it — Runner.storeThroughOperand — so an empty
// one, one that will not evaluate and one before the first element say what
// `read 'b[…]'` says in that dialect. Measured 2026-10-28, `-c`, after
// `b=(x y z)`:
//
//	written                      ksh93u+ 2012-08-01              zsh 5.9.2
//	read -A 'b[1/0]' <<< p       read: 1/0: divide by zero, 1,   division by zero,
//	                             b kept, the next line runs      the script ends
//	read -A 'b[@]' <<< 'p q'     read: @: arithmetic syntax      ( p q )
//	                             error, 1, b kept
//	read -A 'b[-9]' <<< p        read: b: subscript out of       ( p x y z )
//	                             range, 1, b kept
//	read -A 'b[]' <<< p          (p y z) — element 0             not an identifier: b[]
//
// An element holding an array of its own, in the dialect that nests, takes
// the words as that array — `b[3]=(o1 o2); read -A 'b[3]' <<< p` leaves
// `([3]=(p) )`, replaced and not written over in place as a table's key is —
// and asks nothing. Otherwise the axis says where the words go.
func (r *Runner) readArrayIntoAnIndexedElement(array string, elems []string, gaveUp *int) bool {
	base, _, sub, ok := r.subscriptOperandParts(array, r.outputOperandBracketsAreLexed(array))
	if !ok || !isPlainName(base) {
		return false
	}
	// The store is speaking from here on and not the builtin, as in
	// storeThroughOperand: zsh writes `zsh:1: division by zero` with no
	// `read` in it.
	builtin := r.inBuiltin
	r.inBuiltin = r.keptBuiltinLocation(builtin)
	defer func() { r.inBuiltin = builtin }()
	// Ahead of the table's refusal, as for the scalar store: measured
	// 2026-10-28 on zsh 5.9.2, `typeset -A m; read -A 'm[]'` is `not an
	// identifier: m[]`.
	if st, refused := r.storeOperandEmptySubscript(base, array, sub, builtin); refused {
		*gaveUp = st
		return true
	}
	splices := r.sem().SubscriptedArrayLiteral == SubscriptedArrayLiteralSplices
	nests := r.sem().SubscriptedArrayLiteral == SubscriptedArrayLiteralNests
	if r.assocDeclared(base) {
		// A table's key nests in the dialect that nests —
		// readArrayIntoATableKey, asked ahead of this — and is the slice
		// refusal in the one that splices, as its literal is: measured
		// 2026-10-27 on zsh 5.9.2, `typeset -A m; m[k]=1; read -A 'm[k]'` is
		// `m: attempt to set slice of associative array`.
		if !splices {
			return false
		}
		r.readArraySplicedIntoAnElement(base, sub, elems)
		return true
	}
	if r.wholeArrayOperandTakesTheWords(sub) {
		// No element is named, so the axis is not asked.
		r.readArraySplicedIntoAnElement(base, sub, elems)
		return true
	}
	// Read once, and before the axis only where an element can hold an array
	// of its own, which has to be looked at first and is not the question.
	idx, evaluated := 0, false
	if nests {
		if idx, ok = r.readArrayElementIndex(builtin, sub, gaveUp); !ok {
			return true
		}
		evaluated = true
		if pos, within := r.elemPos(r.arrayForWrite(base), idx); within && r.elementHoldsAnArray(base, pos) {
			r.readArrayNestedAt(base, pos, elems)
			return true
		}
	}
	if r.ask(r.sem().ReadArrayIntoAnElementRefillsTheArray,
		"`read -A` into an indexed array's element dropping the element and filling from the first") {
		if !evaluated {
			if idx, ok = r.readArrayElementIndex(builtin, sub, gaveUp); !ok {
				return true
			}
		}
		r.readArrayRefilled(builtin, base, sub, idx, elems, gaveUp)
		return true
	}
	if r.unspecified {
		*gaveUp = r.status
		return true
	}
	// The words land where the element literal `b[i]=(words)` lands them in
	// this dialect.
	if nests {
		pos, within := r.elemPos(r.arrayForWrite(base), idx)
		if !within {
			*gaveUp = r.readArrayElementOutOfRange(builtin, base, sub)
			return true
		}
		r.readArrayNestedAt(base, pos, elems)
		return true
	}
	r.readArraySplicedIntoAnElement(base, sub, elems)
	return true
}

// readArrayElementIndex reads the operand's subscript as the scalar store
// through it reads one, and gives up where that store gives up.
func (r *Runner) readArrayElementIndex(builtin, sub string, gaveUp *int) (int, bool) {
	idx, err := r.subscriptValueOfReference(sub)
	if err != nil {
		*gaveUp = r.readArrayElementGivesUp(builtin, r.sem().BadSubscriptToAnOutputOperand,
			"how much a store through a builtin's operand gives up for an unevaluable subscript",
			r.subscriptFailure(sub, err))
		return 0, false
	}
	return idx, true
}

// readArrayElementOutOfRange is a subscript before the first element, refused
// as the scalar store through the operand refuses it.
func (r *Runner) readArrayElementOutOfRange(builtin, base, sub string) int {
	return r.readArrayElementGivesUp(builtin, r.sem().OutOfRangeElementToAnOutputOperand,
		"how much a store through a builtin's operand gives up for a subscript out of range",
		Wording(r.diag().BadArraySubscript, "%[1]s[%[2]s]: bad array subscript", base, sub))
}

// readArrayElementGivesUp writes a refusal about the operand's subscript in
// the store's words, and gives up as much as the policy says — the same door,
// sentence and status as Runner.storeThroughOperand's.
func (r *Runner) readArrayElementGivesUp(builtin string, p BadSubscriptPolicy, what, refusal string) int {
	return r.storeRefusalStatus(builtin, r.badSubscriptGivesUp(p, what,
		Wording(r.diag().StoreOperandBadSubscript, "%[2]s", builtin, refusal)))
}

// elementHoldsAnArray reports whether the element at pos holds an array of
// its own.
func (r *Runner) elementHoldsAnArray(base string, pos int) bool {
	arr, held := r.arraysLookup(base)
	return held && arr.Has(pos) && arr.Get(pos).Nested != nil
}

// readArrayNestedAt makes the element at pos an array of the words, replacing
// whatever it held.
func (r *Runner) readArrayNestedAt(base string, pos int, elems []string) {
	nested := NewArray(len(elems))
	for i, e := range elems {
		nested.Set(r.arrayBase()+i, Scalar(e))
	}
	arr := r.arrayForWrite(base).clone()
	arr.Set(pos, Element{Nested: nested})
	r.storeArray(base, arr)
}

// readArrayRefilled is the refill: the element goes, and the words fill the
// array from its first element on, over whatever stood there, with the rest
// kept. A name holding a string is the one-element array it would be promoted
// to, which the first word then writes over; a subscript past the end names
// no element to take out, and one before the first is refused.
func (r *Runner) readArrayRefilled(builtin, base, sub string, idx int, elems []string, gaveUp *int) {
	arr := r.arrayForWrite(base).clone()
	if _, held := r.arraysLookup(base); !held {
		if v, isScalar := r.getVar(base); isScalar {
			arr.Set(0, Scalar(v))
		}
	}
	pos, within := r.elemPos(arr, idx)
	if !within {
		*gaveUp = r.readArrayElementOutOfRange(builtin, base, sub)
		return
	}
	if arr.Has(pos) {
		arr.Delete(pos)
	}
	for i, e := range elems {
		arr.Set(i, Scalar(e))
	}
	r.storeArray(base, arr)
}

// readArraySplicedIntoAnElement lands the words exactly where the element
// literal `b[sub]=(words)` lands them in the dialect that splices, by the
// literal's own code, so a span, a subscript before the first element, `0`,
// and a name holding a string or a table read the same here as there. The
// whole-array spelling names every element, as `read 'b[@]'` does: measured
// 2026-10-28 on zsh 5.9.2, `-c`, after `b=(x y z)`:
//
//	read -A 'b[2,3]' <<< 'p q'    ( x p q ), as b[2,3]=(p q)
//	read -A 'b[0]' <<< p          b: assignment to invalid subscript range
//	read -A 'b[-5]' <<< p         ( p x y z )
//	read -A 'b[*]' <<< 'p q'      ( p q )
//
// The refusals are the store's sentences and not the builtin's — no `read:`
// in front of them — and end the script, which readStoreTheArray turns into
// the whole-array store's status.
func (r *Runner) readArraySplicedIntoAnElement(base, sub string, elems []string) {
	a := &syntax.Assign{Name: base}
	if r.wholeArrayOperandTakesTheWords(sub) {
		if r.spliceTargetIsAnArray(a) {
			r.setArray(base, elems)
		}
		return
	}
	r.spliceWordsIntoElement(a, func() string { return sub }, func() ([]string, bool) { return elems, true })
}

// wholeArrayOperandTakesTheWords reports whether `read -A 'b[@]'` names the
// whole array in this dialect, as `read 'b[@]'` does: see
// Semantics.StoreOperandWholeArraySubscript. Where the spelling is an
// expression instead, the arithmetic refuses it as it refuses the scalar's.
func (r *Runner) wholeArrayOperandTakesTheWords(sub string) bool {
	return wholeArraySubscript(sub) &&
		r.sem().StoreOperandWholeArraySubscript == StoreOperandWholeArraySubscriptNamesEveryElement
}

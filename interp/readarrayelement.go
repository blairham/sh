// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// `read -A` into an indexed array's element: the two shells with the letter
// land the words in different places. See
// Semantics.ReadArrayIntoAnElementRefillsTheArray, where the rows are (#6869).

// readArrayIntoAnIndexedElement is `read -A 'b[i]'` where `b` is not a table,
// and reports whether it stored or refused. An element holding an array of
// its own, in the dialect that nests, takes the words as that array — the
// row `b[3]=(o1 o2); read -A 'b[3]' <<< p` leaves `([3]=(p) )`, replaced and
// not written over in place as a table's key is. Otherwise the axis says
// where the words go.
func (r *Runner) readArrayIntoAnIndexedElement(array string, elems []string) bool {
	base, _, sub, ok := r.subscriptOperandParts(array, r.outputOperandBracketsAreLexed(array))
	if !ok || !isPlainName(base) {
		return false
	}
	if r.assocDeclared(base) {
		// A table's key nests in the dialect that nests — readArrayIntoATableKey,
		// asked ahead of this — and is the slice refusal in the one that
		// splices: measured 2026-10-27 on zsh 5.9.2, `typeset -A m; m[k]=1;
		// read -A 'm[k]'` is `m: attempt to set slice of associative array`.
		if r.sem().SubscriptedArrayLiteral != SubscriptedArrayLiteralSplices {
			return false
		}
		r.readArrayStoreRefused(base, Wording(r.diag().SliceOfAnAssociativeArray,
			"%[1]s: attempt to set slice of associative array", base))
		return true
	}
	idx, err := r.subscriptValue(sub)
	if err != nil {
		r.fatal("%s\n", r.subscriptFailure(sub, err))
		return true
	}
	if r.sem().SubscriptedArrayLiteral == SubscriptedArrayLiteralNests {
		if arr, held := r.arraysLookup(base); held {
			if pos, within := r.elemPos(arr, idx); within && arr.Has(pos) && arr.Get(pos).Nested != nil {
				nested := NewArray(len(elems))
				for i, e := range elems {
					nested.Set(r.arrayBase()+i, Scalar(e))
				}
				arr = arr.clone()
				arr.Set(pos, Element{Nested: nested})
				r.storeArray(base, arr)
				return true
			}
		}
	}
	if r.ask(r.sem().ReadArrayIntoAnElementRefillsTheArray,
		"`read -A` into an indexed array's element dropping the element and filling from the first") {
		// The element goes, and the words fill the array from its first
		// element on, over whatever stood there; the rest is kept. A name
		// holding a string is the one-element array it would be promoted
		// to, which the first word then writes over.
		arr := r.arrayForWrite(base).clone()
		if _, held := r.arraysLookup(base); !held {
			if v, isScalar := r.getVar(base); isScalar {
				arr.Set(0, Scalar(v))
			}
		}
		if pos, within := r.elemPos(arr, idx); within && arr.Has(pos) {
			arr.Delete(pos)
		}
		for i, e := range elems {
			arr.Set(i, Scalar(e))
		}
		r.storeArray(base, arr)
		return true
	}
	if r.unspecified {
		return true
	}
	// The words land where the element literal `b[i]=(words)` lands them in
	// this dialect: a splice over the element, padded out to it past the end,
	// and refused over a name holding a string.
	if _, isArray := r.arrayElemsOfTheName(base); !isArray {
		if _, held := r.getVar(base); held {
			r.readArrayStoreRefused(base, Wording(r.diag().ArrayValueToNonArray,
				"%[1]s: attempt to assign array value to non-array", base))
			return true
		}
	}
	cur, _ := r.arrayElemsOfTheName(base)
	pos, within := r.elemPos(r.arrayForWrite(base), idx)
	if !within {
		return false
	}
	out := append([]string{}, cur...)
	for len(out) <= pos {
		out = append(out, "")
	}
	out = append(append(append([]string{}, out[:pos]...), elems...), out[pos+1:]...)
	r.setArray(base, out)
	return true
}

// readArrayStoreRefused reports the splice's own refusal from inside `read`
// the way the freeze's is reported there: as the store's sentence and not the
// builtin's — zsh writes `zsh:1: z: attempt to assign array value to
// non-array` with no `read:` in it, measured 2026-10-27 — and ending the
// script where the dialect ends it for a refused write of the builtin's own.
func (r *Runner) readArrayStoreRefused(name, sentence string) {
	fatal := r.ask(r.sem().ReadonlyRefusalInABuiltinIsFatal,
		"a builtin's refused write to its own output parameter ending the script")
	r.reportFreezeRefusal(name, assignedByBuiltin, fatal, sentence)
}

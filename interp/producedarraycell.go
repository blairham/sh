// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// The array an `unset` reaches, for a name whose elements are produced.
//
// A produced array is in none of the tables a listing or a write walks — that
// is what makes it produced — so every question asked of `Runner.Arrays`
// answers *no* for one, and an `unset` of one of its elements did nothing at
// all, silently, at status 0. Measured 2026-09-26 on zsh 5.9.2 under `-f`,
// against this shell's `argv`, which is the name that dialect gives the
// positional parameters:
//
//	                       reference     here, before
//	unset 'argv[2]'        x  z          x y z
//	unset 'argv[2,3]'      x             x y z
//	unset 'argv[@]'        one empty     x y z
//
// with `a=(x y z)` answering the reference's column in both shells, which is
// what says the construct works and only the route into a produced name does
// not. It is the same noun as #4614, which fixed the four places an
// *assignment* asks the table, and a different seam: this is the removal.
//
// **A producer with no writer is left exactly as it was**, and that is
// measured rather than cautious: bash's `DIRSTACK` is produced and cannot be
// assigned to, and `unset 'DIRSTACK[2]'` there is silent at 0 with the stack
// whole. A removal accepted into a copy nothing reads back is worse than one
// refused, so the copy is only made where there is somewhere to put it.

// writableArrayCell is the array a name holds, in the shape the element rules
// already work on, for a name those rules can actually write back to.
//
// The second result is the answer to "is there an array here to act on", and
// it is deliberately false for a produced array with no writer: that name has
// elements to read and nowhere to put a change, so the caller's "not an
// array" branch — which is silence and 0 for every shell measured — is the
// right one.
//
// The copy is a copy on purpose. `storeArray` is what puts it back, so the
// producer hears about the change through the one chokepoint every other
// write to the name goes through, rather than through a second door of this
// function's own.
func (r *Runner) writableArrayCell(name string) (Array, bool) {
	if a, stored := r.Arrays[name]; stored {
		return a, true
	}
	produce, produced := r.DynamicArrays[name]
	if !produced {
		return nil, false
	}
	if _, writable := r.dynamicArrayWriters[name]; !writable {
		return nil, false
	}
	elems := produce(r)
	a := NewArray(len(elems))
	for i, v := range elems {
		a.Set(i, Scalar(v))
	}
	return a, true
}

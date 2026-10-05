// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"iter"
	"math/bits"
	"slices"
)

// IndexedArray is the store behind an [Array]: a position to an element, with
// no promise that the positions run without gaps.
//
// Reached only through its methods, so that what it is made of can change
// without every reader of an array changing with it (#6099). The methods that
// only read are safe on a nil array, which reads as one with no elements, the
// way a nil map did; the ones that write need an array that exists, the way a
// nil map refused a write.
//
// # Representation
//
// Nearly every array a shell holds is positions 0 to n-1 with none missing,
// and nearly every write to one is an append. So the store is a slice indexed
// by position, which makes a read, a length and an append O(1) and a walk
// O(n) in order with no sort. It was a map until #6099, which made `a+=(x)`
// quadratic — each append searched the whole map for its end — and `${a[i]}`
// on a large array pay for a hash and, at most call sites, a scan.
//
// The shape is not promised, though: `a=(x); a[5]=y` is two elements, at 0
// and 5, in every dialect (see [Array]). A gap no wider than the slice already
// is stays in the slice, with a bitmap saying which positions hold an element;
// the bitmap is nil while none is missing, which is the common case and costs
// nothing. A position below zero, or one so far past the end that the slice
// would be mostly holes — bash's `a[1000000]=x` — moves the whole array into a
// map, which is what it was before, and it stays there. Every method answers
// the same in either form; only the cost differs.
type IndexedArray struct {
	// dense holds positions 0 to len(dense)-1 while sparse is nil. The last
	// one is always held, so len(dense) is one past the highest position.
	dense []Element
	// missing has a bit set for each position in dense that holds nothing,
	// and is nil while no position is missing.
	missing []uint64
	// n is how many positions in dense hold an element.
	n int
	// sparse is the whole store once the shape has stopped fitting a slice,
	// and nil before that.
	sparse map[int]Element
}

// denseGapSlack is how far past the end a write may land and stay in the
// slice even on a small array: `a[10]=x` on an empty zsh array is ordinary
// and should not cost a map.
const denseGapSlack = 64

// NewArray is an empty array with room for n elements, and never nil — the
// spelling of what `make(Array, n)` was.
func NewArray(n int) Array {
	return &IndexedArray{dense: make([]Element, 0, n)}
}

// ArrayOf is an array holding elems at positions 0 to len(elems)-1.
func ArrayOf(elems ...Element) Array {
	return &IndexedArray{dense: slices.Clone(elems), n: len(elems)}
}

// Len is how many positions hold an element.
func (a *IndexedArray) Len() int {
	switch {
	case a == nil:
		return 0
	case a.sparse != nil:
		return len(a.sparse)
	}
	return a.n
}

// heldAt reports whether position i of dense holds an element. i must be in
// range.
func (a *IndexedArray) heldAt(i int) bool {
	return a.missing == nil || a.missing[i>>6]&(1<<(uint(i)&63)) == 0
}

// Get is the element at pos, and the zero element where nothing is there.
func (a *IndexedArray) Get(pos int) Element {
	e, _ := a.Lookup(pos)
	return e
}

// Lookup is the element at pos and whether one is there.
func (a *IndexedArray) Lookup(pos int) (Element, bool) {
	switch {
	case a == nil:
		return Element{}, false
	case a.sparse != nil:
		e, ok := a.sparse[pos]
		return e, ok
	case pos < 0 || pos >= len(a.dense) || !a.heldAt(pos):
		return Element{}, false
	}
	return a.dense[pos], true
}

// Has reports whether pos holds an element.
func (a *IndexedArray) Has(pos int) bool {
	_, ok := a.Lookup(pos)
	return ok
}

// Set puts e at pos.
func (a *IndexedArray) Set(pos int, e Element) {
	if a.sparse != nil {
		a.sparse[pos] = e
		return
	}
	switch end := len(a.dense); {
	case pos >= 0 && pos < end:
		if !a.heldAt(pos) {
			a.missing[pos>>6] &^= 1 << (uint(pos) & 63)
			a.n++
			a.dropMissingIfFull()
		}
		a.dense[pos] = e
	case pos == end:
		a.dense = append(a.dense, e)
		a.n++
		a.growMissing()
	case pos > end && pos-end <= max(end, denseGapSlack):
		if a.missing == nil {
			a.missing = make([]uint64, (end+63)>>6)
		}
		for i := end; i < pos; i++ {
			a.dense = append(a.dense, Element{})
			a.growMissing()
			a.missing[i>>6] |= 1 << (uint(i) & 63)
		}
		a.dense = append(a.dense, e)
		a.n++
		a.growMissing()
	default:
		a.toSparse()
		a.sparse[pos] = e
	}
}

// growMissing keeps the bitmap long enough to cover every position in dense,
// where there is a bitmap. A new word starts with every bit clear, which is
// "held" — so the caller sets the bits of the positions it leaves empty.
func (a *IndexedArray) growMissing() {
	if a.missing != nil && len(a.missing) < (len(a.dense)+63)>>6 {
		a.missing = append(a.missing, 0)
	}
}

// dropMissingIfFull lets the bitmap go once no position is missing, which puts
// the array back on the path that never reads it.
func (a *IndexedArray) dropMissingIfFull() {
	if a.missing != nil && a.n == len(a.dense) {
		a.missing = nil
	}
}

// toSparse moves the store into the map, for a shape a slice would hold
// badly.
func (a *IndexedArray) toSparse() {
	m := make(map[int]Element, a.n+1)
	for i, e := range a.dense {
		if a.heldAt(i) {
			m[i] = e
		}
	}
	a.sparse, a.dense, a.missing, a.n = m, nil, nil, 0
}

// Delete takes whatever is at pos away. Deleting nothing is not an error.
func (a *IndexedArray) Delete(pos int) {
	switch {
	case a == nil:
		return
	case a.sparse != nil:
		delete(a.sparse, pos)
		return
	case pos < 0 || pos >= len(a.dense) || !a.heldAt(pos):
		return
	}
	a.dense[pos] = Element{}
	a.n--
	if pos == len(a.dense)-1 {
		// The last position is always held, so a delete there takes the
		// holes in front of it as well.
		end := pos
		for end > 0 && !a.heldAt(end-1) {
			end--
		}
		a.dense = a.dense[:end]
		if a.missing != nil {
			for i := end; i <= pos; i++ {
				a.missing[i>>6] &^= 1 << (uint(i) & 63)
			}
			a.missing = a.missing[:(end+63)>>6]
		}
		a.dropMissingIfFull()
		return
	}
	if a.missing == nil {
		a.missing = make([]uint64, (len(a.dense)+63)>>6)
	}
	a.missing[pos>>6] |= 1 << (uint(pos) & 63)
}

// All is every position held and its element, lowest position first.
//
// Writing to the array while walking it is allowed: a position deleted before
// the walk reaches it is not visited, and one added past the walk's place may
// or may not be, the way it was for a map.
func (a *IndexedArray) All() iter.Seq2[int, Element] {
	return func(yield func(int, Element) bool) {
		switch {
		case a == nil:
			return
		case a.sparse != nil:
			for _, k := range a.subscripts() {
				// Asked again rather than read from a snapshot, so a
				// delete made by the loop body is honored.
				if e, ok := a.Lookup(k); ok && !yield(k, e) {
					return
				}
			}
			return
		}
		i := 0
		for ; i < len(a.dense) && a.sparse == nil; i++ {
			if a.heldAt(i) && !yield(i, a.dense[i]) {
				return
			}
		}
		if a.sparse == nil {
			return
		}
		// The loop body wrote a shape the slice could not hold, which moved
		// the store into the map; the positions not yet visited are there.
		for _, k := range a.subscripts() {
			if k < i {
				continue
			}
			if e, ok := a.Lookup(k); ok && !yield(k, e) {
				return
			}
		}
	}
}

// subscripts returns the assigned subscripts, in order.
func (a *IndexedArray) subscripts() []int {
	out := make([]int, 0, a.Len())
	switch {
	case a == nil:
	case a.sparse != nil:
		for k := range a.sparse {
			out = append(out, k)
		}
		slices.Sort(out)
	default:
		for i := range a.dense {
			if a.heldAt(i) {
				out = append(out, i)
			}
		}
	}
	return out
}

// bounds is the lowest and highest subscript assigned, and whether there are
// any.
//
// O(1) in the slice, whose last position is always held and whose first
// nearly always is; a scan in the map, as it always was.
func (a *IndexedArray) bounds() (lo, hi int, any bool) {
	switch {
	case a == nil:
		return 0, 0, false
	case a.sparse != nil:
		for k := range a.sparse {
			if !any || k < lo {
				lo = k
			}
			if !any || k > hi {
				hi = k
			}
			any = true
		}
		return lo, hi, any
	case len(a.dense) == 0:
		return 0, 0, false
	case a.missing == nil:
		return 0, len(a.dense) - 1, true
	}
	for w, word := range a.missing {
		if held := ^word; held != 0 {
			return w<<6 + bits.TrailingZeros64(held), len(a.dense) - 1, true
		}
	}
	// Unreachable: the last position is always held.
	return len(a.dense) - 1, len(a.dense) - 1, true
}

// shallowClone is a copy whose elements are the same values, nested arrays
// shared — what maps.Clone made of the map. clone is the deep one.
func (a *IndexedArray) shallowClone() Array {
	if a == nil {
		return nil
	}
	out := &IndexedArray{n: a.n}
	if a.sparse != nil {
		out.sparse = make(map[int]Element, len(a.sparse))
		for k, v := range a.sparse {
			out.sparse[k] = v
		}
		return out
	}
	out.dense = slices.Clone(a.dense)
	out.missing = slices.Clone(a.missing)
	return out
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "iter"

// IndexedArray is the store behind an [Array]: a position to an element, with
// no promise that the positions run without gaps.
//
// Reached only through its methods, so that what it is made of can change
// without every reader of an array changing with it (#6099). The methods that
// only read are safe on a nil array, which reads as one with no elements, the
// way a nil map did; the ones that write need an array that exists, the way a
// nil map refused a write.
type IndexedArray struct {
	m map[int]Element
}

// NewArray is an empty array with room for n elements, and never nil — the
// spelling of what `make(Array, n)` was.
func NewArray(n int) Array {
	return &IndexedArray{m: make(map[int]Element, n)}
}

// ArrayOf is an array holding elems at positions 0 to len(elems)-1.
func ArrayOf(elems ...Element) Array {
	a := NewArray(len(elems))
	for i, e := range elems {
		a.Set(i, e)
	}
	return a
}

// Len is how many positions hold an element.
func (a *IndexedArray) Len() int {
	if a == nil {
		return 0
	}
	return len(a.m)
}

// Get is the element at pos, and the zero element where nothing is there.
func (a *IndexedArray) Get(pos int) Element {
	if a == nil {
		return Element{}
	}
	return a.m[pos]
}

// Lookup is the element at pos and whether one is there.
func (a *IndexedArray) Lookup(pos int) (Element, bool) {
	if a == nil {
		return Element{}, false
	}
	e, ok := a.m[pos]
	return e, ok
}

// Has reports whether pos holds an element.
func (a *IndexedArray) Has(pos int) bool {
	_, ok := a.Lookup(pos)
	return ok
}

// Set puts e at pos.
func (a *IndexedArray) Set(pos int, e Element) {
	a.m[pos] = e
}

// Delete takes whatever is at pos away. Deleting nothing is not an error.
func (a *IndexedArray) Delete(pos int) {
	if a == nil {
		return
	}
	delete(a.m, pos)
}

// All is every position held and its element. The order is not promised, the
// way a map's was not; a caller that wants one sorts, or asks subscripts.
//
// Writing to the array while walking it is allowed, the way it was for a map:
// a position deleted before the walk reaches it is not visited.
func (a *IndexedArray) All() iter.Seq2[int, Element] {
	return func(yield func(int, Element) bool) {
		if a == nil {
			return
		}
		for k, v := range a.m {
			if !yield(k, v) {
				return
			}
		}
	}
}

// shallowClone is a copy whose elements are the same values, nested arrays
// shared — what maps.Clone made of the map. clone is the deep one.
func (a *IndexedArray) shallowClone() Array {
	if a == nil {
		return nil
	}
	out := NewArray(len(a.m))
	for k, v := range a.m {
		out.m[k] = v
	}
	return out
}

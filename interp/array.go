// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"sort"
	"strings"

	"github.com/blairham/sh/syntax"
)

// arrayBase is the index the first element answers to. bash and ksh93 count
// from 0 and zsh from 1, which is measured — and it means a subscript cannot
// be used as a slice offset without asking.
func (r *Runner) arrayBase() int {
	if r.ask(r.sem().ArrayBaseIsZero, "arrays being indexed from zero") {
		return 0
	}
	return 1
}

// Array is an indexed array: a subscript to a value, and no promise that the
// subscripts run without gaps.
//
// A map rather than a list because that is what an array *is* in two of the
// three shells that have them: `a=(x); a[5]=y` leaves an array of two
// elements with subscripts 0 and 5, not six elements four of which are empty.
// Storing it as a list made the padding real, and everything that counts or
// lists an array counted it — `${#a[@]}` said 6, and `for i in "${!a[@]}"`
// visited four subscripts nobody had assigned.
//
// The third shell reads the same store the other way, walking the whole
// extent and finding an unassigned subscript empty. So the storage is sparse
// in every dialect and only the *reading* is a question.
type Array map[int]string

// subscripts returns the assigned subscripts, in order.
func (a Array) subscripts() []int {
	out := make([]int, 0, len(a))
	for k := range a {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

// extent is the range a dense reading walks: the base up to the highest
// subscript assigned.
func (a Array) extent(base int) (from, to int) {
	subs := a.subscripts()
	if len(subs) == 0 {
		return base, base - 1
	}
	return base, subs[len(subs)-1]
}

// pastTheEnd is the position one past the highest subscript assigned — where
// an append lands, and what a negative subscript counts back from.
func (a Array) pastTheEnd() int {
	_, hi := a.extent(0)
	return hi + 1
}

// The subscripts stored are *positions*, counted from zero whatever the
// dialect counts from. The base belongs at the edges — where a script writes
// a subscript and where one is read back — and not in the store, because
// `a=(one two)` says nothing about which number the first element answers to
// and must not have to ask.
func (r *Runner) setArray(name string, elems []string) {
	a := make(Array, len(elems))
	for i, v := range elems {
		a[i] = v
	}
	r.storeArray(name, a)
}

// storeArray puts an array back and keeps the scalar view in step.
func (r *Runner) storeArray(name string, a Array) {
	if r.Arrays == nil {
		r.Arrays = map[string]Array{}
	}
	r.Arrays[name] = a
	// A plain `$a` has to keep working. The first element is stored rather
	// than the scalar view, because *which* view it is depends on a dialect
	// and building an array must not need one: getVar asks, and only when
	// the answer could differ.
	//
	// Which element this is cannot be seen from a script, and saying so is
	// worth more than a test that looks as though it checks: with more than
	// one element `$a` is answered by the array itself and never reaches
	// here, and with one there is nothing to choose between. It is the
	// lowest subscript because that is what it means, not because anything
	// could tell.
	if subs := a.subscripts(); len(subs) > 0 {
		r.setVar(name, a[subs[0]])
	} else {
		r.setVar(name, "")
	}
}

// setArrayElem assigns one element. Any subscript at or above the base is
// legal, whether or not anything below it has been assigned, and a negative
// one counts back from the end.
func (r *Runner) setArrayElem(name string, idx int, value string) {
	a := r.Arrays[name]
	if a == nil {
		a = Array{}
	}
	// The subscript's meaning is the dialect's — `a[5]` is the sixth element
	// in one shell and the fifth in another — so this is where the base is
	// asked, at the edge where a script wrote a number. A negative subscript
	// asks nothing: `a[-1]=x` replaces the last element in all three shells
	// with arrays, the one whose subscripts count from 1 included, so the
	// base plays no part in it. Measured against a sparse array, the end it
	// counts from is one past the highest *subscript*, not the element count.
	pos := idx - r.arrayBase()
	if idx < 0 {
		pos = a.pastTheEnd() + idx
	}
	if pos < 0 {
		r.diagf("%s[%d]: index out of range\n", name, idx)
		return
	}
	a[pos] = value
	r.storeArray(name, a)
}

// appendArray adds elements after the highest subscript.
func (r *Runner) appendArray(name string, elems []string) {
	a := r.Arrays[name]
	if a == nil {
		a = Array{}
	}
	next := 0
	if subs := a.subscripts(); len(subs) > 0 {
		next = subs[len(subs)-1] + 1
	}
	for i, v := range elems {
		a[next+i] = v
	}
	r.storeArray(name, a)
}

// unsetArrayElem removes one subscript.
//
// Removed and not blanked, in every dialect: the dense reading finds an
// unassigned subscript empty on its own, so `unset a[1]` leaves a hole in one
// shell and an empty element in another out of the same store.
func (r *Runner) unsetArrayElem(name string, idx int) {
	a, ok := r.Arrays[name]
	if !ok {
		return
	}
	pos := idx - r.arrayBase()
	if idx < 0 {
		// `unset "a[-1]"` removes the last element — the same end-relative
		// reading every other subscript position takes, and just as unanimous.
		pos = a.pastTheEnd() + idx
	}
	delete(a, pos)
	r.storeArray(name, a)
}

// arrayScalar is what a plain `$a` gives when `a` is an array.
//
// Two answers: every element joined by a space, or the first element alone.
// Asked only when there is more than one element, because with none or one the
// two agree — and only when a scalar is actually read, because building an
// array is not a question about how it would be flattened.
func (r *Runner) arrayScalar(elems []string) string {
	switch {
	case len(elems) == 0:
		return ""
	case len(elems) == 1:
		return elems[0]
	case r.ask(r.sem().ArrayScalarIsTheWholeArray, "a plain `$a` giving the whole array"):
		return strings.Join(elems, " ")
	default:
		return elems[0]
	}
}

// arrayElems returns an array's elements as the dialect reads them, treating a
// plain variable as a one-element array — which is what makes `x=v; echo
// ${x[0]}` work.
//
// Two readings of one store. A sparse reading yields the assigned elements and
// nothing else; a dense one walks the whole extent and yields an empty string
// where nothing was assigned. Two of the shells with arrays read it the first
// way and one the second, so the dialect answers.
func (r *Runner) arrayElems(name string) ([]string, bool) {
	// Produced first, for the same reason a produced scalar is read ahead of
	// the stored table: the record is the answer, and a copy left in Arrays
	// would be the previous pipeline's.
	if elems, ok := r.pipelineStatuses(name); ok {
		return elems, true
	}
	if a, ok := r.Arrays[name]; ok {
		return r.readArray(a), true
	}
	// Produced rather than stored, and asked after the stored table so that
	// a script assigning to the name gets its own value back — the same
	// order the scalar ones follow.
	if produce, ok := r.DynamicArrays[name]; ok {
		return produce(r), true
	}
	if v, ok := r.getVar(name); ok {
		return []string{v}, true
	}
	return nil, false
}

// readArray is the elements a dialect sees.
func (r *Runner) readArray(a Array) []string {
	subs := a.subscripts()
	// Asked only where the two readings differ, which is when something is
	// missing between the base and the highest subscript. A contiguous array
	// reads the same either way, and that is almost every array there is.
	if !r.arrayHasGaps(a) {
		out := make([]string, 0, len(subs))
		for _, k := range subs {
			out = append(out, a[k])
		}
		return out
	}
	if r.ask(r.sem().ArraysAreSparse, "an unassigned subscript being no element at all") {
		out := make([]string, 0, len(subs))
		for _, k := range subs {
			out = append(out, a[k])
		}
		return out
	}
	from, to := a.extent(0)
	out := make([]string, 0, to-from+1)
	for i := from; i <= to; i++ {
		out = append(out, a[i])
	}
	return out
}

// arrayKeys is the subscripts a dialect sees, which `${!a[@]}` yields.
func (r *Runner) arrayKeys(a Array) []int {
	if !r.arrayHasGaps(a) || r.ask(r.sem().ArraysAreSparse, "an unassigned subscript being no element at all") {
		return a.subscripts()
	}
	from, to := a.extent(0)
	out := make([]int, 0, to-from+1)
	for i := from; i <= to; i++ {
		out = append(out, i)
	}
	return out
}

// arrayHasGaps reports whether anything between the base and the highest
// subscript was never assigned — the only case the two readings differ in.
func (r *Runner) arrayHasGaps(a Array) bool {
	from, to := a.extent(0)
	return to-from+1 != len(a)
}

// arraySubscript answers `${a[i]}`, `${a[@]}` and `${a[*]}`.
//
// The whole-array forms are the reason this returns a slice: `"${a[@]}"` is
// one field per element, exactly as `"$@"` is one per parameter, and joining
// them would lose an element that contains a space.
func (r *Runner) arraySubscript(e *syntax.ParamExpr) ([]string, bool) {
	if e.Index == nil {
		return nil, false
	}
	if a, ok := r.AssocArrays[e.Name]; ok {
		// The attribute decides the subscript's reading before anything is
		// looked up: a declared name takes it as a key, an undeclared one
		// falls through to the numeric path below.
		return r.assocSubscript(a, e), true
	}
	elems, ok := r.arrayElems(e.Name)
	if !ok {
		return nil, true
	}
	switch idx := r.subscriptText(e.Index); idx {
	case "@", "*":
		return elems, true
	default:
		n, err := r.parseNum(idx)
		if err != nil {
			return nil, true
		}
		if v, ok := r.elemAt(e.Name, elems, n); ok {
			return []string{v}, true
		}
		return nil, true
	}
}

// elemAt answers a numeric subscript against the elements a dialect read.
//
// A subscript at or above the base counts from the base, which is the
// dialect's. A negative one counts back from the end and asks nothing:
// `${a[-1]}` is the last element in all three shells with arrays, the one
// whose subscripts count from 1 included, so the base plays no part in it.
//
// Measured against a sparse array, the end is one past the highest
// *subscript*, not the element count: with subscripts 0 and 5, `${a[-1]}` is
// the element at 5 and `${a[-2]}` is the unassigned 4 — nothing — rather than
// the element at 0. A subscript out of range in either direction is no
// element at all, exactly as `${a[9]}` on three elements is.
func (r *Runner) elemAt(name string, elems []string, n int) (string, bool) {
	// When the sparse reading compacted a gap out of elems, a position can no
	// longer be counted there; the store still holds every position. Only a
	// *stored* array can be behind elems here — a produced one is read before
	// the table, in the same order arrayElems reads them.
	a, stored := r.Arrays[name]
	if _, produced := r.pipelineStatuses(name); produced {
		stored = false
	}
	compacted := stored && len(elems) != a.pastTheEnd()

	var pos int
	if n < 0 {
		end := len(elems)
		if compacted {
			end = a.pastTheEnd()
		}
		pos = end + n
	} else {
		pos = n - r.arrayBase()
	}
	if pos < 0 {
		return "", false
	}
	if compacted {
		v, ok := a[pos]
		return v, ok
	}
	if pos >= len(elems) {
		return "", false
	}
	return elems[pos], true
}

// subscriptText reads a subscript without letting it expand as a pattern.
//
// `${a[*]}` is the whole array and `${a[@]}` is its elements, and the two were
// behaving differently for a reason that had nothing to do with either: the
// subscript went through ordinary expansion, where `*` is a pattern that
// matched no file and became the empty string. `@` is not a pattern, so it
// survived and `*` did not.
//
// A subscript written as a plain literal is taken as written. Anything else —
// `${a[$i]}`, `${a[i+1]}` — still expands, because it has to.
func (r *Runner) subscriptText(w *syntax.Word) string {
	if w != nil && len(w.Spans) == 1 && w.Spans[0].Kind == syntax.Literal {
		return strings.TrimSpace(w.Spans[0].Value)
	}
	return strings.TrimSpace(r.joinWord(w))
}

// arrayElementCount reports how many elements a name holds and whether it is
// an array at all, either kind.
func (r *Runner) arrayElementCount(name string) (int, bool) {
	if a, ok := r.Arrays[name]; ok {
		return len(r.readArray(a)), true
	}
	if m, ok := r.AssocArrays[name]; ok {
		return len(m), true
	}
	return 0, false
}

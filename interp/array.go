// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"errors"
	"sort"
	"strconv"
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

// elemPos resolves a subscript as written to a position in the store.
//
// The subscript's meaning is the dialect's — `a[5]` is the sixth element in
// one shell and the fifth in another — so this is where the base is asked, at
// the edge where a script wrote a number. A negative subscript asks nothing:
// `a[-1]=x` replaces the last element in all three shells with arrays, the one
// whose subscripts count from 1 included, so the base plays no part in it.
// Measured against a sparse array, the end it counts from is one past the
// highest *subscript*, not the element count.
func (r *Runner) elemPos(a Array, idx int) (int, bool) {
	if idx < 0 {
		pos := a.pastTheEnd() + idx
		return pos, pos >= 0
	}
	pos := idx - r.arrayBase()
	return pos, pos >= 0
}

// setArrayElem assigns one element. Any subscript at or above the base is
// legal, whether or not anything below it has been assigned, and a negative
// one counts back from the end.
//
// sub is the subscript as it was written, because that is what one dialect's
// refusal names — `a[x-2]: bad array subscript`, not the `-1` it came to.
//
// A subscript that lands *before* the first element is refused, and the
// refusal ends the script at 1 in every shell measured. It used to be a
// wording of our own at status 0 with the script running on, which is the
// silent half of a wrong answer: the element was not written and the next
// command read the array as though it had been.
//
// Which subscript reaches this is the array base and nothing else. Where the
// first element is 0 it is a negative one that counts back past the start;
// where the first element is 1 it is `a[0]`, which is below it. Neither shell
// has both, and that is why one rule needs two spellings to show it.
func (r *Runner) setArrayElem(name string, idx int, sub, value string) {
	a := r.Arrays[name]
	if a == nil {
		a = Array{}
	}
	pos, ok := r.elemPos(a, idx)
	if !ok {
		if idx < 0 && r.ask(r.sem().NegativeSubscriptPastTheStartInserts,
			"a negative subscript past the first element placing one in front of it") {
			r.storeArray(name, insertAtTheFront(a, value))
			return
		}
		if r.unspecified {
			return
		}
		r.fatal("%s\n", Wording(r.diag().BadArraySubscript,
			"%[1]s[%[2]s]: bad array subscript", name, sub))
		return
	}
	a[pos] = value
	r.storeArray(name, a)
}

// insertAtTheFront puts value before every element there is, moving the rest
// up one.
//
// The answer one shell gives a negative subscript that runs past the start:
// however far past, it lands in front and the array grows by exactly one, so
// `a=(p q)` takes `a[-3]`, `a[-4]` and `a[-5]` to the same place. The others
// refuse it, which is the axis rather than this arithmetic.
func insertAtTheFront(a Array, value string) Array {
	out := make(Array, len(a)+1)
	for _, pos := range a.subscripts() {
		out[pos+1] = a[pos]
	}
	out[0] = value
	return out
}

// appendArrayElem is `a[i]+=v`: the value joins what the element already
// holds rather than replacing it.
//
// A different operation from `a+=(v)`, which adds an element after the last
// one, and the two are told apart by the subscript alone. Measured unanimous
// in the three shells with arrays, the one that counts from 1 included — so
// the base is asked here exactly as it is for a plain element assignment and
// nothing else about the form is a question.
//
// An unset element has nothing to append to, so the same spelling stores the
// value as it stands: `a=(x y); a[5]+=Q` leaves `Q` at subscript 5, not an
// error and not an empty string joined to anything.
func (r *Runner) appendArrayElem(name string, idx int, sub, value string) {
	if pos, ok := r.elemPos(r.Arrays[name], idx); ok {
		value = r.Arrays[name][pos] + value
	}
	r.setArrayElem(name, idx, sub, value)
}

// unsetArrayElem takes one subscript away, or blanks it where it stands.
//
// Which of those it is comes from UnsetArraySpan, the same field that answers
// `unset a[@]`, because in the shell that blanks they are one rule rather than
// two: `unset` of a span replaces the span with a single empty element, and a
// single subscript is a span of one. It is invisible in the middle of an array
// — a removed subscript reads back empty under a dense reading anyway — and
// visible at the end, where removing shrinks the extent and blanking does not.
// A three-element array unset at its last element was coming back with two
// elements where that shell has three, so every later count and every append
// landed one place early.
//
// The removing reading needs no distinction between the middle and the end:
// the dense reader finds an unassigned subscript empty on its own, so one
// store serves the shell that leaves a hole and the shell that sees an empty
// element.
//
// A subscript that lands *before* the first element is refused, and the
// answer is the status the builtin carries rather than the end of the script.
// That is the boundary an assignment already refuses — see setArrayElem —
// reached from `unset` instead, and the same two spellings show it is one
// rule: where the first element is 1, `a[0]` is below it, and where it is 0,
// only a negative subscript counting back past the start can be. Neither
// shell has both. It was silent at status 0 here, so a script that asked to
// remove something out of reach was told it had.
//
// Which spelling a shell can reach is UnsetArraySpan again rather than a new
// question. The blanking reading acts only on a span that is there, so a
// negative subscript that ran past the start finds nothing to replace and is
// left alone without a word; the removing readings count from 0, so no
// non-negative subscript is below their first element.
//
// sub is the subscript as it was written, because one dialect's refusal
// quotes it back — `[x-9]`, not the -9 it came to.
func (r *Runner) unsetArrayElem(name string, idx int, sub string) int {
	blanks := r.unsetBlanksInPlace()
	a, isArray := r.Arrays[name]
	if !isArray {
		// A name holding nothing at all has no first element for a subscript
		// to be before, and is left alone without a word everywhere. A scalar
		// has one under the blanking reading, where a span of one is what
		// `unset a[i]` means and a scalar is such a span — so `a=v; unset
		// "a[0]"` reaches the boundary there. The removing readings answer a
		// scalar without reading the subscript at all, which is
		// UnsetNotAnArray's question rather than this one.
		if _, held := r.getVar(name); blanks && held && idx >= 0 && idx < r.arrayBase() {
			return r.refuseSubscriptToUnset(name, sub)
		}
		return 0
	}
	pos, within := r.elemPos(a, idx)
	if blanks {
		// Only a subscript that names an element already there is blanked.
		// One past the end has no span to replace, and the array is left as
		// it was rather than gaining an element — `a=(x y z); unset a[9]`
		// keeps three.
		//
		// Measured, and it is why a negative subscript is asked about
		// separately: of the negative ones only `-1` acts in the blanking
		// shell, so `unset a[-2]` on `(x y z)` leaves all three where the
		// removing shells take the middle one away.
		if idx < 0 && idx != -1 {
			return 0
		}
		if !within && idx >= 0 {
			return r.refuseSubscriptToUnset(name, sub)
		}
		if _, held := a[pos]; !held {
			return 0
		}
		a[pos] = ""
		r.storeArray(name, a)
		return 0
	}
	if !within {
		return r.refuseSubscriptToUnset(name, sub)
	}
	delete(a, pos)
	r.storeArray(name, a)
	return 0
}

// refuseSubscriptToUnset reports a subscript `unset` could not reach because
// it lands before the array's first element, and answers with the status the
// builtin carries.
//
// The refusal leaves the array exactly as it was and lets the next command
// run, which is where it parts from the assignment's: that one ends the
// script. A script can therefore test it, so the status is returned rather
// than thrown.
//
// The location must not name the builtin. The dialect that puts a builtin's
// name in the prefix does not put it here — this is its parameter store
// speaking rather than `unset` — and the two that do name it put the name in
// the sentence, where they put every other builtin's. Same split as
// badSubscriptToUnset.
func (r *Runner) refuseSubscriptToUnset(name, sub string) int {
	outer := r.inBuiltin
	r.inBuiltin = ""
	defer func() { r.inBuiltin = outer }()
	r.diagf("%s\n", Wording(r.diag().UnsetSubscriptBeforeTheFirstElement,
		"unset: [%[2]s]: bad array subscript", name, sub))
	return 1
}

// unsetWholeArray is `unset a[@]`, where the subscript names every element
// rather than one of them. It reports whether the spelling was handled, so
// the dialect that has no such reading can go on to the expression it does
// have.
//
// The two spellings that reach it were doing nothing at all. `@` is not an
// arithmetic expression, so the subscript failed to evaluate and the element
// nobody named was quietly not removed — `unset a[@]` on a full array left
// every element in place at status 0, where two of the three shells with
// arrays empty it. Silent, and the shape scripts use to start a list over.
func (r *Runner) unsetWholeArray(name string) (handled bool, code int) {
	switch r.unsetArraySpan() {
	case UnsetArraySpanRemovesTheElements:
		if _, ok := r.Arrays[name]; ok {
			r.storeArray(name, Array{})
			return true, 0
		}
		// A name that is no array is not emptied, and the two cases part
		// here: one holding a value is complained about and reported as a
		// failure, and one holding nothing at all is left alone without a
		// word. Both keep what they had, which is why neither writes to the
		// store.
		if _, held := r.getVar(name); held {
			r.diagf("%s\n", Wording(r.diag().UnsetNotAnArray,
				"unset: %[1]s: not an array variable", name))
			return true, 1
		}
		return true, 0
	case UnsetArraySpanLeavesOneEmptyElement:
		// The span becomes one empty string, which is the same rule this
		// shell applies to a single element — `unset a[2]` blanks it in place
		// — read over every element at once. A scalar is one element by that
		// reading and comes back empty; a name holding nothing has no span,
		// and nothing is what it keeps. An array that is already empty has no
		// span either, so `a=(); unset a[@]` does not gain an element.
		if a, ok := r.Arrays[name]; ok {
			if len(a) > 0 {
				r.storeArray(name, Array{0: ""})
			}
			return true, 0
		}
		if _, held := r.getVar(name); held {
			r.setVar(name, "")
		}
		return true, 0
	case UnsetArraySpanIsAnExpression:
		return false, 0
	default:
		// Unspecified, already reported by name. Refusing is not "quietly do
		// nothing", which is the bug this function exists to fix.
		return true, r.status
	}
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
		// An expression, not a numeral: `${a[1+1]}` and `${a[i+1]}` name the
		// element `${a[2]}` names. It took a numeral and nothing else, so
		// every other spelling silently expanded to nothing.
		n, ok := r.subscriptIndex(idx)
		if !ok {
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

// subscriptValue evaluates a subscript, which is an arithmetic expression and
// not only a numeral: `${a[1+1]}`, `a[i]=v` and `unset a[i+1]` all name the
// element a bare `2` names.
//
// One function for every path that reads a subscript as a number, because the
// four of them disagreed while each had a reading of its own. Writing through
// one evaluated — since the array literal learned to place its elements — and
// reading one back, assigning through `${a[i]:=v}` and unsetting one all took
// a numeral and nothing else, so `a[1+1]=v` stored where `${a[1+1]}` could not
// look. Two spellings of the same subscript naming two different elements is
// the sharpest form of the silent wrong answer, because the script that writes
// and reads with the same expression sees an array that forgets what it stored.
//
// A numeral is answered without building a parser for it. That is what every
// caller used to do and is still the common case, so the arithmetic evaluator
// is reached only by a subscript that needs it.
//
// An unset name is 0 rather than an error, which is what the evaluator says
// about any bare name — `${a[k]}` with `k` unset is the first element in every
// shell on the panel that has arrays.
func (r *Runner) subscriptValue(text string) (int, error) {
	text = strings.TrimSpace(text)
	if n, err := strconv.Atoi(text); err == nil {
		return n, nil
	}
	tree, err := r.arithTree(nil, text)
	if err != nil {
		return 0, err
	}
	return r.evalArith(tree)
}

// subscriptFailure is the sentence a dialect writes about a subscript that
// would not evaluate, whichever half of the reading refused it.
//
// A subscript can fail twice over — the text may not parse as an expression,
// or it may parse and not evaluate — and the panel words the two differently
// for `$(( ))` already. The same two wordings serve here, because a subscript
// is an expression and every shell measured says about `${a[b c]}` exactly
// what it says about `$((b c))`.
func (r *Runner) subscriptFailure(text string, err error) string {
	var se *syntax.Error
	if errors.As(err, &se) {
		switch se.Kind {
		case syntax.ErrArithOperand, syntax.ErrArithOperandEnd, syntax.ErrArithOperator, syntax.ErrArithBadOperator:
			// Blamed on the text the caller names rather than on the text the
			// parser was handed, which are the same everywhere but one.
			return r.diag().arithParseFailure(se, text)
		}
		return r.diag().ParseFailure(err)
	}
	return r.arithFailure(text, err)
}

// subscriptIndex evaluates a subscript and reports a failure where every
// shell in the panel reports one, abandoning the word.
//
// The reporting is what was missing. A subscript that would not evaluate was
// answered with an error nobody read: `${a[b c]}` expanded to nothing at
// status 0 and the script carried on, where all four shells write a
// diagnostic, give up on the command, and exit non-zero. An empty string is a
// plausible value for a real element, so nothing downstream could tell.
//
// The status is the ordinary fatal one rather than the failed-expansion one:
// bash draws that line itself, exiting 1 under `-c` for a bad expression where
// `${x@QQ}` from the same invocation exits 127.
func (r *Runner) subscriptIndex(text string) (int, bool) {
	n, err := r.subscriptValue(text)
	if err != nil {
		r.diagf("%s\n", r.subscriptFailure(text, err))
		r.expandErr = true
		return 0, false
	}
	return n, true
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

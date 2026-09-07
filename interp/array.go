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
	// The unique attribute is applied here rather than at each of the
	// half-dozen callers, because it is a property of the name that holds
	// for every write there is: measured, `typeset -U a=(1 1 2)` dedupes,
	// `a+=(2 4 4)` dedupes, and `a[2]=3` dedupes the whole array and not
	// only the element written. One choke point is what makes those one
	// rule instead of three that can drift apart.
	if r.unique[name] {
		a = r.uniqueElems(a)
	}
	// And what the name's other attributes make of each element, for the
	// same reason and at the same one place: a write is a write however it
	// was spelled, so an element assignment, an append and a literal all
	// come here and all fold. See compoundElemsFolded.
	a = r.compoundElemsFolded(name, a)
	if r.unspecified {
		return
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
	// And the tied scalar, if this array is half of a tie — the *other*
	// name, which the line above is not: that one keeps `$a` answering for
	// `a` itself. See tiedscalar.go.
	r.mirrorArrayToScalar(name, r.readArray(a))
}

// uniqueElems is what `typeset -U` leaves of an array: the first occurrence
// of each value, in the order the first occurrences stand.
//
// Measured against zsh 5.9.2, which is the one shell in the panel with the
// letter. The *first* occurrence is the one kept and it does not move:
// `b=(1 2 3); b=(3 $b)` reads back `3 1 2`, so the new leading `3` stays
// where it was written and the old one is the copy that goes. Dedupe happens
// at write time and not at read: `typeset -U c=(1 2 3 2)` already reads `1 2
// 3` with the attribute removed afterwards.
//
// The array is read the way the dialect reads it and stored back dense,
// because that is what the measurement says: `typeset -U f=(1 2); f[5]=1`
// leaves three elements — `1`, `2` and one empty — where the same lines
// without the letter leave five. The two empties the gap made are elements
// like any others and dedupe to one, and the trailing `1` is the copy that
// goes. Reading first is what makes that true; deduping the store's
// subscripts instead would have left two elements and no empty at all.
func (r *Runner) uniqueElems(a Array) Array {
	elems := r.readArray(a)
	out := make(Array, len(elems))
	seen := make(map[string]bool, len(elems))
	pos := 0
	for _, e := range elems {
		if seen[e] {
			continue
		}
		seen[e] = true
		out[pos] = e
		pos++
	}
	return out
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

// markIndexed gives a name the indexed-array attribute, which is what
// `declare -a` and `typeset -a` do. Declaring twice keeps the elements.
//
// The attribute *is* the store, exactly as it is for a table — see markAssoc —
// so an empty array is what a name declared without a value holds. Almost
// nothing needed that before, because an element assignment brings an array
// into being on its own; what needs it is the one question a store with
// nothing in it cannot otherwise answer, which is whether the name is an array
// holding nothing or a scalar holding nothing.
//
// A declared table is left alone rather than replaced. Two letters naming two
// kinds of array on one line is a shape nothing here has measured, and taking
// the table away on the strength of a guess would lose its elements.
func (r *Runner) markIndexed(name string) {
	if _, produced := r.DynamicArrays[name]; produced {
		return
	}
	if r.assocDeclared(name) {
		return
	}
	if _, ok := r.Arrays[name]; ok {
		return
	}
	if r.Arrays == nil {
		r.Arrays = map[string]Array{}
	}
	r.Arrays[name] = Array{}
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
		return r.unsetScalarElem(name, idx, sub)
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

// unsetSubscriptRange is `unset "a[lo,hi]"` where the dialect reads the comma
// as the separator of a range rather than as the arithmetic operator whose
// value is its right operand.
//
// handled is false where the subscript is not a pair, and where the two
// readings name the same thing — a pair whose ends are the same subscript is
// that subscript either way — so the single reading answers it and no axis is
// asked. That is the discipline the expansion side already follows for
// `${a[2,2]}`.
//
// Measured on zsh 5.9.2, the one shell that reads ranges, with `a=(x y z)`:
//
//	a[1,2]    [][z]        the span becomes one empty element
//	a[1,3]    []           every element, and one is left
//	a[0,1]    [][y][z]     a start below the first is the first
//	a[3,4]    [x][y][]     an end past the last is the last
//	a[4,5]    [x][y][z]    a start past the last names nothing
//	a[2,1]    [x][][y][z]  a span with nothing in it is an empty element
//	                       *inserted* where it would have begun
//	a[-1,-1]  [x][y][]     the last element
//	a[-2,-1]  [x][y][z]    a negative start other than -1 acts on nothing
//	a[0,0]    refused      the whole span is below the first element
//
// The last two rows would not have been guessed. The negative rule is the
// array's alone — a *string* loses characters to every negative within reach —
// and it is the same "only -1 acts" this shell's single subscript already
// follows. The refusal is the span rule spanIsBelowTheFirstElement holds, and
// it is why `a[0]` is refused where `a[0,1]` is not: one names a span wholly
// out of reach, the other one that begins out of reach and ends inside.
func (r *Runner) unsetSubscriptRange(name, sub string) (handled bool, code int) {
	lo, hi, ok := splitSubscriptRange(sub)
	if !ok {
		return false, 0
	}
	from, errLo := r.subscriptValue(lo)
	to, errHi := r.subscriptValue(hi)
	if errLo == nil && errHi == nil && from == to {
		// The same subscript at both ends is that subscript under either
		// reading, so there is nothing to ask and nothing to do differently.
		return false, 0
	}
	if !r.ask(r.sem().SubscriptCommaIsARange, "`${a[1,3]}` naming a range rather than one subscript") {
		return false, 0
	}
	if errLo != nil {
		return true, r.badSubscriptToUnset(lo, errLo)
	}
	if errHi != nil {
		return true, r.badSubscriptToUnset(hi, errHi)
	}
	if r.spanIsBelowTheFirstElement(from, to) {
		return true, r.refuseSubscriptToUnset(name, sub)
	}
	if a, isArray := r.Arrays[name]; isArray {
		return true, r.unsetElementSpan(name, a, from, to)
	}
	v, held := r.getVar(name)
	if !held {
		// Neither an element nor a character for any span to reach.
		return true, 0
	}
	if r.scalarUnsetReadsAsCharacters(v, sub) {
		return true, r.unsetCharacterSpan(name, v, from, to)
	}
	// The element reading, where a scalar is the one element at the base: a
	// span that reaches it takes the whole name away, exactly as the single
	// subscript naming it does, and one that does not is the question every
	// other subscript on a scalar asks. No shell measured reads both a range
	// and a scalar-as-element, so this is the two readings composed rather
	// than a column of its own.
	if first, tail, within := r.spanOver(1, from, to); within && tail > first {
		r.unsetName(name)
		return true, 0
	}
	return true, r.refuseSubscriptOnAScalar(name)
}

// spanIsBelowTheFirstElement says whether every subscript a span could name is
// before the array's first element.
//
// One rule for the single subscript and for the pair, which is what tells them
// apart rather than a check on each: `a[0]` is the span `[0,0]` where the
// first element is 1, and is refused; `a[0,1]` begins out of reach and ends
// inside, and is not. A negative end never counts as below — it is counted
// back from the end and reaches nothing rather than reaching before the start,
// which is the silence `array/unsetting-past-the-start` records.
func (r *Runner) spanIsBelowTheFirstElement(from, to int) bool {
	base := r.arrayBase()
	return from >= 0 && from < base && to >= 0 && to < base
}

// spanOver resolves a written range against n units and reports whether the
// span begins anywhere the units reach.
//
// The endpoints are subscripts and take the two rules a single subscript
// takes: counted from the dialect's base, or back from the end when negative.
// A start below the first unit is the first, an end past the last is the last,
// and an end before the start leaves the span empty *at* the start — which is
// where an empty element is inserted rather than nothing happening at all.
func (r *Runner) spanOver(n, from, to int) (first, tail int, within bool) {
	base := r.arrayBase()
	first = from - base
	if from < 0 {
		first = n + from
	}
	last := to - base
	if to < 0 {
		last = n + to
	}
	if first < 0 {
		first = 0
	}
	if last >= n {
		last = n - 1
	}
	if first >= n {
		// The span starts past the last unit: nothing to replace, and
		// nothing beyond the end to put an empty one in front of.
		return first, first, false
	}
	tail = last + 1
	if tail < first {
		tail = first
	}
	return first, tail, true
}

// unsetElementSpan is what `unset` does to the elements a range names.
func (r *Runner) unsetElementSpan(name string, a Array, from, to int) int {
	blanks := r.unsetBlanksInPlace()
	if from < 0 && from != -1 && blanks {
		// Only the last element answers to a negative subscript in the shell
		// that blanks, and a range's start follows the same rule its single
		// subscript does. Measured: `unset "a[-2,-1]"` leaves all three
		// elements where `unset "a[-1,-1]"` blanks the last.
		return 0
	}
	n := a.pastTheEnd()
	first, tail, within := r.spanOver(n, from, to)
	if !within {
		return 0
	}
	elems := make([]string, n)
	for pos, v := range a {
		if pos >= 0 && pos < n {
			elems[pos] = v
		}
	}
	out := make([]string, 0, n+1)
	out = append(out, elems[:first]...)
	if blanks {
		// The span becomes a single empty element, which is this shell's
		// reading of `unset` over a span, read across a range rather than one
		// element at a time. A span with nothing in it still becomes one, so
		// a reversed range *inserts*.
		out = append(out, "")
	}
	out = append(out, elems[tail:]...)
	r.setArray(name, out)
	return 0
}

// unsetCharacterSpan takes the characters a range names out of a string.
//
// The same span over characters, and the empty-span case says nothing here:
// an empty character inserted leaves the string as it was, so `unset "a[3,2]"`
// is a no-op where the same reversed range on an array gains an element.
func (r *Runner) unsetCharacterSpan(name, v string, from, to int) int {
	chars := r.units(v)
	first, tail, within := r.spanOver(len(chars), from, to)
	if !within {
		return 0
	}
	r.setVar(name, strings.Join(chars[:first], "")+strings.Join(chars[tail:], ""))
	return 0
}

// unsetScalarElem is `unset "a[i]"` where the name is not an array.
//
// The panel gives three answers and each one falls out of what a subscripted
// name *means* where the name holds a string, rather than being a rule of its
// own. Measured on `a=hello`:
//
//	                bash 5.3   bash 3.2   ksh93     zsh
//	unset a[0]      unset      refused    unset     refused
//	unset a[1]      refused    refused    silent    ello
//	unset a[2]      refused    refused    silent    hllo
//	unset a[-1]     refused    refused    silent    hell
//	unset a[-5]     refused    refused    silent    ello
//	unset a[-6]     refused    refused    silent    hello
//	unset a[9]      refused    refused    silent    hello
//
// Where a subscript names a *character* the string loses it, and nothing else
// about the name changes: past the end names nothing, and below the first
// character is the boundary every subscript below the first meets. Every
// negative within reach acts, which an array under the same reading does not —
// there only `-1` does — so the string is a character position and not a
// one-element array wearing one.
//
// Where it names an *element*, a scalar is the one element at the base. The
// subscript that names it takes the whole name away — not the value, the name,
// attribute and all — which is unanimous among the shells that read it that
// way. Every other subscript names nothing, and there the two part:
// UnsetSubscriptOnAScalarIsAnError.
//
// A name holding nothing at all has neither an element nor a character for a
// subscript to name, and is left alone without a word everywhere — which is
// what keeps `unset b[0]` on a name nobody set quiet.
func (r *Runner) unsetScalarElem(name string, idx int, sub string) int {
	v, held := r.getVar(name)
	if !held {
		return 0
	}
	if r.scalarUnsetReadsAsCharacters(v, sub) {
		return r.unsetScalarCharacter(name, v, idx, sub)
	}
	if idx == r.arrayBase() {
		// The subscript names the scalar itself, so this is `unset a` written
		// the long way round.
		r.unsetName(name)
		return 0
	}
	return r.refuseSubscriptOnAScalar(name)
}

// refuseSubscriptOnAScalar is what one dialect does about a subscript that
// names no element of a name that is no array, and what the other does not.
func (r *Runner) refuseSubscriptOnAScalar(name string) int {
	if !r.ask(r.sem().UnsetSubscriptOnAScalarIsAnError,
		"a subscript naming no element of a name that is not an array") {
		return 0
	}
	outer := r.inBuiltin
	r.inBuiltin = ""
	defer func() { r.inBuiltin = outer }()
	r.diagf("%s\n", Wording(r.diag().UnsetNotAnArray,
		"unset: %[1]s: not an array variable", name))
	return 1
}

// unsetScalarCharacter takes one character out of a string.
//
// The two span policies coincide here and neither is asked: replacing a span
// of one character with a single empty element and removing that character
// leave the same string, because an empty character is nothing at all.
func (r *Runner) unsetScalarCharacter(name, v string, idx int, sub string) int {
	chars := r.units(v)
	pos := idx - r.arrayBase()
	if idx < 0 {
		pos = len(chars) + idx
	}
	if pos < 0 {
		if idx < 0 {
			// A negative that reaches back past the first character names
			// nothing and says nothing — measured, and not the same answer
			// as the non-negative below: `a=hello; unset "a[-6]"` is quiet
			// where `unset "a[0]"` is refused. Negative subscripts never meet
			// the boundary, here or on an array.
			return 0
		}
		// Below the first character, which is the boundary a subscript below
		// the first element meets — the same refusal, reached through a
		// string.
		return r.refuseSubscriptToUnset(name, sub)
	}
	if pos >= len(chars) {
		// Past the end names nothing, and nothing is what changes.
		return 0
	}
	r.setVar(name, strings.Join(append(append([]string{}, chars[:pos]...), chars[pos+1:]...), ""))
	return 0
}

// scalarUnsetReadsAsCharacters is scalarReadsAsCharacters for `unset`, where
// what the two readings differ about is not a value but what is left behind.
//
// Asked wherever the name holds a string, because the readings agree nowhere:
// one takes a character out and the other takes the whole name away or
// complains. The one-character string that makes the *expansion* readings
// agree does not make these agree either — `a=v; unset "a[1]"` leaves an empty
// string where the base is 1, and takes the name away or refuses where it is
// 0, which is three different outcomes from one line.
//
// An empty string is asked too. It has no character for any subscript to name,
// so the character reading leaves it alone — and the element reading still has
// the one element a scalar is, and takes the name away through it. Reading the
// empty string as "no characters, so no question" took `a=; unset "a[1]"` from
// an empty name to no name at all.
func (r *Runner) scalarUnsetReadsAsCharacters(string, string) bool {
	return r.ask(r.sem().ScalarSubscriptIsACharacter, "`${s[2]}` naming a character of a string")
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

// arrayBareName is what a plain `$a` reads when `a` holds an array, and
// whether the name is set at all. Both halves are one question, because the
// axis that chooses the reading decides them together.
//
// Where a bare name is the whole list, the name is set whenever the array is
// — an array with no elements included, which reads as the empty string.
//
// Where a bare name is one element it **is** `${a[base]}`: the element at the
// base position, and not the lowest subscript that happens to be assigned. A
// name whose base element was removed, or was never written, is therefore
// unset while the array still holds elements, and reading the lowest assigned
// subscript instead is the bug this replaces — `a=(x y z); unset "a[0]"` gave
// `y` where every shell that reads one element gives nothing.
//
// Measured 2026-09-06, from a file and through `-c` alike, `env -i` with a
// scratch HOME. On `a=(x y z); unset "a[0]"`, bash 5.3.15, bash as `sh`, bash
// 3.2.57 and ksh93u+ all leave `${a-U}` at `U`, `"$a"` empty, `${#a}` at 0,
// `$((a+5))` at 5 and `set -u; $a` an unbound variable, while `${a[@]}` still
// yields `y z` and `${a[@]+S}` is still set. `a[5]=q` with no base element
// ever written reads the same way, so it is the position and not the removal
// that decides. ksh93's `set -u` complaint names `a[0]` rather than `a`,
// which says the reading out loud. zsh asks nothing here: it reads the list,
// and its own `unset "a[1]"` leaves an empty element in place rather than a
// gap.
func (r *Runner) arrayBareName(a Array) (string, bool) {
	base, assigned := a[0]
	// The two readings agree while the base element is the only element there
	// is — which is `a=(x)` and every scalar-shaped array a script builds —
	// so the axis is asked only where they part.
	if assigned && len(a) == 1 {
		return base, true
	}
	if r.ask(r.sem().ArrayScalarIsTheWholeArray, "a plain `$a` giving the whole array") {
		// Joined with the first character of IFS, exactly as `$*` is: an
		// empty array joins to the empty string, and the name is set.
		return strings.Join(r.readArray(a), ifsFirst(r.ifs())), true
	}
	return base, assigned
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
		// Joined with the first character of IFS, exactly as `$*` is and as
		// `${a[*]}` already was. A hard space was wrong for the same reason
		// it would be wrong there: measured, `IFS=-; a=(x y z); echo "$a"`
		// is `x-y-z` and `IFS=; echo "$a"` is `xyz`.
		return strings.Join(elems, ifsFirst(r.ifs()))
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
	if e.IndexFlags != nil {
		// Before the associative reading and before the target is chosen: a
		// flag group decides how the subscript is *read*, so a name whose
		// attribute would take it as a key has to be told the group is there
		// rather than handed `(re)value` to look up.
		if v, handled := r.flaggedSubscript(e); handled {
			return v, true
		}
	}
	if a, ok := r.assocFor(e.Name); ok {
		// The attribute decides the subscript's reading before anything is
		// looked up: a declared name takes it as a key, an undeclared one
		// falls through to the numeric path below.
		return r.assocSubscript(a, e), true
	}
	elems, scalar, ok := r.subscriptTarget(e)
	if !ok {
		return nil, true
	}
	return r.subscriptOver(e, subscriptSource{name: e.Name, elems: elems, scalar: scalar})
}

// subscriptSource is what a subscript reaches into: the values it counts
// through, and whether those are one string read as characters rather than a
// list of elements.
//
// It exists because a subscript is written on two different things. A
// *parameter* supplies one, and so does the result of an expansion —
// `${${(f)x}[2]}` counts through fields that no name holds. Everything below
// this line asks about the values rather than about where they came from,
// which is why the name is carried here rather than read off the node again:
// it is empty for an expansion's result, and the one thing that reads it —
// elemAt — is asking whether a *stored* sparse array compacted a gap out of
// the elements, which an expansion's result has none of.
type subscriptSource struct {
	name   string
	elems  []string
	scalar bool
}

// subscriptOver answers a subscript against values already in hand.
//
// Every reading below is the source's rather than a name's, so the same code
// answers `${a[2]}` and the subscript on a nested expansion's result. The
// scalar flag is what the character reading needs, and it says nothing about
// how many values there are: an array holding one element is not a scalar,
// and reading it as characters would be wrong however short it is.
func (r *Runner) subscriptOver(e *syntax.ParamExpr, src subscriptSource) ([]string, bool) {
	elems, scalar := src.elems, src.scalar
	idx := r.subscriptText(e.Subscript())
	if r.wholeArrayIndex(e) {
		return elems, true
	}
	if lo, hi, isRange := splitSubscriptRange(idx); isRange {
		return r.rangeSubscript(src, idx, lo, hi)
	}
	if at, extra := topLevelComma(idx); at >= 0 && extra &&
		r.sem().SubscriptCommaIsARange == Yes {
		// A range has two ends. The dialect that reads the comma that way
		// has no reading for a third — measured, `${a[1,2,3]}` is a bad
		// substitution there — and answering with the arithmetic comma's
		// last operand would be the other dialect's reading wearing this
		// one's name.
		r.reportBadSubstitution(e)
		return nil, true
	}
	if scalar && r.scalarReadsAsCharacters(elems[0], idx) {
		if c, ok := r.charAt(elems[0], idx); ok {
			return []string{c}, true
		}
		return nil, true
	}
	// An expression, not a numeral: `${a[1+1]}` and `${a[i+1]}` name the
	// element `${a[2]}` names. It took a numeral and nothing else, so
	// every other spelling silently expanded to nothing.
	n, ok := r.subscriptIndex(idx)
	if !ok {
		return nil, true
	}
	if scalar {
		if v, ok := scalarElemAt(elems[0], n, r.arrayBase()); ok {
			return []string{v}, true
		}
		return nil, true
	}
	if v, ok := r.elemAt(src.name, elems, n); ok {
		return []string{v}, true
	}
	return nil, true
}

// scalarElemAt answers a numeric subscript against a plain string read as an
// array of one, which is what a subscript on a scalar is where it is not read
// as a character.
//
// The base names the string itself and nothing else does — including a
// negative subscript, which is the half this exists for: a string has no end
// to count back from, so `${s[-1]}` on `hello` is no element in bash and in
// ksh93 alike. Both answer empty; bash also says `s: bad array subscript`
// while ksh93 is silent, and the value is what is unanimous.
//
// It answered `hello` before, because the string was handed to the array path
// and a one-element array's last element is its first. A plausible value with
// no diagnostic anywhere near it, and the same silent shape `${a[b c]}` had.
func scalarElemAt(v string, n, base int) (string, bool) {
	if n != base {
		return "", false
	}
	return v, true
}

// subscriptTarget is what a subscript reaches into, and whether that is one
// string rather than a list.
//
// Three sources, because a subscript is written on all three. A name reads its
// array or, failing that, its value as an array of one — which is what makes
// `x=v; echo ${x[0]}` work. `@` and `*` are the positional parameters, as a
// list. Every other special parameter supplies a value, and a value is a
// string: `${?[1]}` is a digit of the status, not an element of anything.
//
// The scalar flag is what the character reading needs and the element reading
// does not, so it is answered here rather than guessed at from the length: an
// array holding one element is not a scalar, and reading it as characters
// would be wrong however short it is.
func (r *Runner) subscriptTarget(e *syntax.ParamExpr) (elems []string, scalar, ok bool) {
	if e.Name == "@" || e.Name == "*" {
		return r.Params, false, true
	}
	if _, isArray := r.Arrays[e.Name]; isArray {
		elems, ok = r.arrayElems(e.Name)
		return elems, false, ok
	}
	if _, produced := r.pipelineStatuses(e.Name); produced {
		elems, ok = r.arrayElems(e.Name)
		return elems, false, ok
	}
	if _, dynamic := r.DynamicArrays[e.Name]; dynamic {
		elems, ok = r.arrayElems(e.Name)
		return elems, false, ok
	}
	if v, held := r.getVar(e.Name); held {
		return []string{v}, true, true
	}
	if v, special := r.specialParam(e); special {
		return []string{v}, true, true
	}
	return nil, false, false
}

// splitSubscriptRange splits `1,3` into its two halves, reporting whether the
// subscript is written as a pair at all.
//
// One comma exactly. Nested parentheses and brackets hold their own commas —
// `${a[f(1,2),3]}` has two halves and not three — and a subscript with two
// top-level commas is a pair in no shell measured, so it is left to the
// arithmetic that reads it as an expression.
func splitSubscriptRange(idx string) (lo, hi string, ok bool) {
	at, extra := topLevelComma(idx)
	if at < 0 || extra {
		return "", "", false
	}
	return idx[:at], idx[at+1:], true
}

// topLevelComma reports where the first comma outside any nesting is, and
// whether another follows it.
func topLevelComma(idx string) (at int, extra bool) {
	depth := 0
	at = -1
	for i := range len(idx) {
		switch idx[i] {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case ',':
			if depth != 0 {
				continue
			}
			if at >= 0 {
				return at, true
			}
			at = i
		}
	}
	return at, false
}

// rangeSubscript answers a subscript written as a pair, `${a[1,3]}`.
//
// Both readings are worked out before either is chosen, and the axis is asked
// only when they disagree — `${a[2,2]}` is the second element whether the
// comma separates a range or joins two expressions, so it needs no answer, and
// neither does a pair either reading refuses.
func (r *Runner) rangeSubscript(src subscriptSource, idx, lo, hi string) ([]string, bool) {
	span, spanOK := r.rangeElems(src.elems, src.scalar, lo, hi)
	whole, wholeErr := r.subscriptValue(idx)
	var one []string
	oneOK := wholeErr == nil
	if oneOK {
		if v, found := r.elemAt(src.name, src.elems, whole); found {
			one = []string{v}
		}
	}
	if spanOK && oneOK && equalStrings(span, one) {
		return span, true
	}
	if r.ask(r.sem().SubscriptCommaIsARange, "`${a[1,3]}` naming a range rather than one subscript") {
		if !spanOK {
			return nil, true
		}
		return span, true
	}
	if !oneOK {
		r.diagf("%s\n", r.subscriptFailure(idx, wholeErr))
		r.expandErr = true
		return nil, true
	}
	return one, true
}

// rangeElems is the range reading of `[lo,hi]`, over elements or characters.
//
// The endpoints are subscripts, so they are counted from the dialect's base
// and from the end when they are negative — the same two rules a single
// subscript follows, which is what keeps `${a[2,2]}` and `${a[2]}` naming the
// same element.
//
// The rest is measured rather than derived, on zsh 5.9.2 with `a=(w x y z)`
// and `s=hello`. An `hi` past the last is the last, so `${a[0,99]}` is the
// whole array, and an `hi` before the first leaves the range empty. An `lo`
// below the base is the first, so `${a[0,2]}` is `w x`.
//
// And the two ends of the panel's one range differ in a place no symmetry
// predicts: an `lo` written as a *negative* that counts back past the first
// element leaves an array empty — `${a[-5,2]}` on four elements is nothing —
// where the same reach past the start of a string is clamped, so `${s[-6,2]}`
// on five characters is still `he`.
func (r *Runner) rangeElems(elems []string, scalar bool, lo, hi string) ([]string, bool) {
	var units []string
	if scalar {
		units = r.units(elems[0])
	} else {
		units = elems
	}
	from, err := r.subscriptValue(lo)
	if err != nil {
		return nil, false
	}
	to, err := r.subscriptValue(hi)
	if err != nil {
		return nil, false
	}
	n := len(units)
	base := r.arrayBase()
	first, negative := from-base, from < 0
	if negative {
		first = n + from
	}
	last := to - base
	if to < 0 {
		last = n + to
	}
	if first < 0 {
		if negative && !scalar {
			return []string{}, true
		}
		first = 0
	}
	if last >= n {
		last = n - 1
	}
	if last < first {
		return []string{}, true
	}
	span := units[first : last+1]
	if scalar {
		return []string{strings.Join(span, "")}, true
	}
	return span, true
}

// scalarReadsAsCharacters asks whether a subscript on a plain string names one
// of its characters, and asks only where the two readings differ.
//
// They agree on a one-character string at the subscript both readings answer
// to, and on any subscript that names nothing under either. Everywhere else
// the readings are two different strings with no diagnostic between them,
// which is why the answer is a dialect's rather than a default.
func (r *Runner) scalarReadsAsCharacters(v, idx string) bool {
	n, err := r.subscriptValue(idx)
	if err != nil {
		// Neither reading has an answer; the element path reports it.
		return false
	}
	c, asChar := r.charAt(v, idx)
	elem, asElem := scalarElemAt(v, n, r.arrayBase())
	if asChar == asElem && c == elem {
		return false
	}
	return r.ask(r.sem().ScalarSubscriptIsACharacter, "`${s[2]}` naming a character of a string")
}

// charAt is one character of a string, counted the way the dialect counts
// subscripts and from the end when the subscript is negative.
//
// What a character *is* is the locale's, not a byte: measured under a UTF-8
// locale, the shell that reads a string this way reports `${s[2]}` of a
// five-character string holding a two-byte character as that character, and
// under `LC_ALL=C` the same shell reports the string's second byte. See
// interp/multibyte.go, which owns both halves of that question.
func (r *Runner) charAt(v, idx string) (string, bool) {
	n, err := r.subscriptValue(idx)
	if err != nil {
		return "", false
	}
	chars := r.units(v)
	var pos int
	if n < 0 {
		pos = len(chars) + n
	} else {
		pos = n - r.arrayBase()
	}
	if pos < 0 || pos >= len(chars) {
		return "", false
	}
	return chars[pos], true
}

// equalStrings compares two readings of one subscript, counting a nil result
// and an empty one as the same: both say the subscript named nothing.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// subscriptIsARange reports whether a subscript was written as a pair and this
// dialect reads it as one. The callers that need it are asking a question
// about the *shape* of the answer — how many fields it makes, and whether its
// length is a count or a width — rather than about its value.
func (r *Runner) subscriptIsARange(e *syntax.ParamExpr) bool {
	if e.Index == nil {
		return false
	}
	if _, _, ok := splitSubscriptRange(r.subscriptText(e.Subscript())); !ok {
		return false
	}
	return r.sem().SubscriptCommaIsARange == Yes
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
//
// Expanded but never *matched*: a subscript is an expression or a key and is
// nothing a directory holds, so the pathname step has no business here. It
// used to run, and a subscript carrying a metacharacter across more than one
// span therefore reported `no matches found` and abandoned the word —
// reachable as soon as a subscript could hold a group, since `(` is a
// metacharacter in the grammar that has both.
func (r *Runner) subscriptText(w *syntax.Word) string {
	if w != nil && len(w.Spans) == 1 && w.Spans[0].Kind == syntax.Literal {
		return strings.TrimSpace(w.Spans[0].Value)
	}
	return strings.TrimSpace(strings.Join(r.expandWordNoSplit(w), ""))
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
		case syntax.ErrArithOperand, syntax.ErrArithOperandEnd, syntax.ErrArithOperator,
			syntax.ErrArithBadOperator, syntax.ErrArithCharacterMissing:
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
	if m, ok := r.assocFor(name); ok {
		return len(m), true
	}
	return 0, false
}

// subscriptYieldsAList reports whether a subscript named several elements
// rather than one value, which is what decides whether `${#…}` is a count or a
// width.
//
// `[@]` and `[*]` always do. A range does when what it ranged over was a list;
// a range over a string is a substring, which is one value however many
// characters it holds.
func (r *Runner) subscriptYieldsAList(e *syntax.ParamExpr) bool {
	if e.Index == nil {
		return false
	}
	if r.wholeArrayIndex(e) {
		return true
	}
	if r.assocSearchSubscript(e) {
		// A search over an association names every key that matched, so the
		// count is the count of matches: measured, `${#m[(I)*]}` on a
		// two-element table is 2 and `${#m[(I)zz]}` is 0.
		return true
	}
	if !r.subscriptIsARange(e) {
		return false
	}
	_, scalar, ok := r.subscriptTarget(e)
	return ok && !scalar
}

// subscriptJoinsElements reports whether a quoted expansion of this subscript
// is one field with the elements joined rather than one field each.
//
// `[*]` is the spelling that says so outright. A range says it by the name it
// was written on: everything but `@` joins, because `@` is the one parameter
// whose fields survive quoting.
func (r *Runner) subscriptJoinsElements(e *syntax.ParamExpr) bool {
	if e.Name == "@" {
		// The name keeps its fields however it is subscripted: measured,
		// `"${@[*]}"` is one field per parameter, where `"${*[1,3]}"` —
		// neither half of it `@` — is one joined field. The other spelling
		// of the list needs no clause of its own: `[@]` is neither `*` nor a
		// range, so it already falls through to one field each.
		return false
	}
	// A search over an association joins for the same reason a range does:
	// it is not `@`. Measured with two keys holding spaces —
	// `set -- "${m[(I)*]}"` is one field holding both, `set -- ${m[(I)*]}`
	// is one field each with the spaces intact, and `"${(@)m[(I)*]}"` is one
	// field each again.
	return r.joinedArrayIndex(e) || r.subscriptIsARange(e) || r.assocSearchSubscript(e)
}

// compoundElemsFolded is what a name's attributes make of an array's elements
// — the compound half of attributeFolded.
//
// A scalar assignment through an attributed name needs no dialect: `typeset
// -i n; n=3+4` is 7 and `typeset -u d; d=again` is AGAIN in every shell that
// spells the letter. An *element* is where the panel splits, so the fold is
// asked for rather than assumed — see
// Semantics.CompoundElementsGoThroughTheAttribute.
//
// Asked only when the name actually carries one of these attributes, so an
// ordinary array never needs a dialect. A value the fold would return
// unchanged is left alone without asking either, which is what keeps an array
// of canonical numbers under `-i` free of the question.
func (r *Runner) compoundElemsFolded(name string, a Array) Array {
	// A fast path and not a rule: attributeWouldChange answers no for every
	// element of a name with none of these attributes, so removing this
	// changes nothing observable — it only stops the loop below from walking
	// every array this shell ever stores.
	if !r.integer[name] && !r.lowered[name] && !r.uppered[name] {
		return a
	}
	changed := false
	for _, sub := range a.subscripts() {
		if r.attributeWouldChange(name, a[sub]) {
			changed = true
			break
		}
	}
	if !changed {
		return a
	}
	if !r.ask(r.sem().CompoundElementsGoThroughTheAttribute,
		"an element written to an attributed name going through the attribute") {
		return a
	}
	return r.foldedElems(name, a)
}

// foldedElems folds every element of an array through the name's attributes,
// with no question asked. Its own function because the two callers ask
// different questions of different dialect fields and then want the same
// arithmetic: a *write* asks CompoundElementsGoThroughTheAttribute, and an
// attribute arriving over a standing array asks CompoundAttribute.
func (r *Runner) foldedElems(name string, a Array) Array {
	folded := Array{}
	for _, sub := range a.subscripts() {
		v, ok := r.attributeFolded(name, a[sub])
		if !ok {
			// The integer evaluation failed and has already said so, which
			// is the one case attributeFolded stores nothing for. The array
			// is left as it stands rather than half rewritten.
			return a
		}
		folded[sub] = v
	}
	return folded
}

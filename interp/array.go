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

// bounds is the lowest and highest subscript assigned, and whether there are
// any.
//
// A scan rather than the two ends of subscripts(), which is the same answer
// for a great deal more work: sorting a store to read one number off the end
// of it was measured at a third of what a real startup spent in this file,
// and every caller of the two below wants a number and not an order.
func (a Array) bounds() (lo, hi int, any bool) {
	for k := range a {
		if !any || k < lo {
			lo = k
		}
		if !any || k > hi {
			hi = k
		}
		any = true
	}
	return lo, hi, any
}

// extent is the range a dense reading walks: the base up to the highest
// subscript assigned.
func (a Array) extent(base int) (from, to int) {
	_, hi, any := a.bounds()
	if !any {
		return base, base - 1
	}
	return base, hi
}

// denseElems is the elements of an array whose subscripts are 0 to n-1 with
// none missing, which is the shape nearly every array in a shell has.
//
// Worth telling apart because that shape needs no sort and no second pass:
// n distinct subscripts, none below zero and none above n-1, can only be
// exactly 0 to n-1, so the position each element goes to is the subscript
// itself. Anything else — a gap, a negative subscript — says no here and
// goes the long way, where the order has to be worked out.
func (a Array) denseElems() ([]string, bool) {
	lo, hi, any := a.bounds()
	if any && (lo != 0 || hi != len(a)-1) {
		return nil, false
	}
	out := make([]string, len(a))
	for k, v := range a {
		out[k] = v
	}
	return out, true
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
	if write, produced := r.dynamicArrayWriters[name]; produced {
		// A *produced* array, whose elements are not this table's to keep:
		// the producer answers ahead of anything stored here, so a write left
		// in the store would be accepted in silence and then read back as
		// whatever the producer says. The one chokepoint every write reaches
		// — a literal, an append, an element, a splice — so the producer
		// hears about all four rather than about whichever one a hook was
		// written beside. See SetDynamicArrayWriter.
		write(r, r.readArray(a))
		return
	}
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
	//
	// assignedAsTheCompoundView and not setVar, because a plain scalar store
	// is what *replaces* an array now — see scalarOverCompound. Without the
	// form this write would ask that question of the array it is the view of,
	// and the answer that keeps the array would send it back through here.
	if lo, _, any := a.bounds(); any {
		r.setVarAs(name, a[lo], assignedAsTheCompoundView)
	} else {
		r.setVarAs(name, "", assignedAsTheCompoundView)
	}
	// And the tied scalar, if this array is half of a tie — the *other*
	// name, which the line above is not: that one keeps `$a` answering for
	// `a` itself. See tiedscalar.go.
	r.mirrorArrayToScalar(name, a)
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
	if r.subscriptSplicesCharacters(name) {
		// The name is holding a string and this dialect's subscript names one
		// of its characters, so the value is spliced in rather than an
		// element being written and the string being lost. Here rather than
		// at the assignment statement because every way of writing an element
		// has to agree: `v[2]=X`, `typeset "v[2]"=X`, `(( v[2] = 5 ))`,
		// `${(P)x::=Z}` with `x` naming `v[2]`, and the descriptor a
		// redirection leaves behind all arrive at this one store, and a rule
		// stated at one of them is a rule the other four contradict.
		r.spliceScalarElem(name, idx, sub, value, false)
		return
	}
	a := r.elementsOfName(name, idx)
	if r.unspecified {
		return
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

// elementsOfName is the array an element write starts from: the one the name
// holds, or the one the scalar it holds becomes.
//
// A subscript on the left of `=` turns a name holding a string into an array,
// and the string it was holding is the first element of it. That was dropped —
// `a=abc; a[1]=x` left `([1]="x")`, the right shape at status 0 with the
// script's own value gone out of it (#1570).
//
// The rule and the helper are the ones `a+=(x)` already followed. The two
// differ only in what made the name an array — the operator there and the
// subscript here — so #1502 fixed one half of this and left the other, which
// is why they are one function now rather than two statements of the same
// thing. The dialect where a subscript on a string names a *character* never
// arrives: subscriptSplicesCharacters answers ahead of both callers, and on
// that side there is no array to promote into.
//
// *That* the value is kept is core — every shell in the panel with arrays
// keeps it. **When** it is kept is not, and the only spelling that can tell
// is a subscript that counts back from the end: `a=abc; a[-1]=x` writes the
// element the promotion just made in bash 5.3 and is `subscript out of range`
// in ksh93, which resolves the subscript against the elements the name has
// and finds none. Hence the axis, asked at that one shape.
func (r *Runner) elementsOfName(name string, idx int) Array {
	if _, stored := r.Arrays[name]; stored {
		return r.arrayForWrite(name)
	}
	if _, produced := r.DynamicArrays[name]; produced {
		// A produced array is an array: its elements are what the write
		// starts from, and there is no scalar underneath it to promote.
		return r.arrayForWrite(name)
	}
	a, _ := r.appendedOverAScalar(name)
	if len(a) == 0 || idx >= 0 {
		return a
	}
	// A negative subscript counts back from the end, so it is the one
	// spelling whose answer depends on whether the promotion has happened
	// yet — and the panel splits on exactly that and nowhere else. Asked
	// here rather than at either caller, and only where there is a scalar to
	// promote and a subscript that counts back over it.
	if !r.ask(r.sem().NegativeSubscriptCountsOverAPromotedScalar,
		"a negative subscript counting back over a scalar a write is promoting") {
		return Array{}
	}
	return a
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
	if r.subscriptSplicesCharacters(name) {
		// A string joins at the span the subscript names rather than at an
		// element: `v=abc; v[2]+=X` is `abXc`, the character that was there
		// with the value after it and back in its place. Ahead of the join
		// below because that one reads an *element* to join to, and a string
		// has none.
		r.spliceScalarElem(name, idx, sub, value, true)
		return
	}
	a := r.elementsOfName(name, idx)
	if r.unspecified {
		return
	}
	if pos, ok := r.elemPos(a, idx); ok {
		// Joined through appendedValue rather than with `+`, because the
		// name's attribute decides which of the two joins this is:
		// `typeset -ia a=(1 2); a[1]+=5` is `7` in bash and ksh93, not `25`.
		// An element the array does not have yet has nothing to join, and
		// setArrayElem's own fold evaluates the value on its way in.
		//
		// Read through elementsOfName so that a scalar being promoted is
		// something to join to: `a=abc; a[0]+=x` is `abcx`, and reading the
		// store directly made it `x` — the promotion happened below, after
		// the join had already decided there was nothing there (#1570).
		v, ok := r.appendedValue(name, a[pos], value)
		if !ok {
			return
		}
		value = v
	}
	r.setArrayElem(name, idx, sub, value)
}

// appendScalarToArray is `a+=x` where `a` is holding an array: the value joins
// the array rather than replacing it with a string.
//
// It replaced it. `a=(1 2); a+=x` left `declare -- a="1x"` in the bash
// dialect and `typeset a='1 2x'` in the zsh one — a plain scalar at status 0,
// with no diagnostic, where every shell in the panel that has arrays leaves an
// array. `assign`'s scalar branch ends by deleting the name's array, which is
// right for `a=x` and wrong for `a+=x`, and the value it joined was the
// *scalar view* of the whole array rather than an element, which is where the
// `1 2x` came from (#1571).
//
// Where the value joins is a real disagreement and is asked rather than
// picked. Measured 2026-09-08 with `a=(1 2); a+=x; typeset -p a`:
//
//	bash 5.3.15         declare -a a=([0]="1x" [1]="2")   n=2
//	bash 5.3.15 as sh   declare -a a=([0]="1x" [1]="2")   n=2
//	bash 3.2.57         declare -a a=([0]="1x" [1]="2")   n=2
//	ksh93               typeset -a a=(1x 2)               n=2
//	zsh 5.9.2           typeset -a a=( 1 2 x )            n=3
//
// Four join the first element and one adds a new one, so it is
// ScalarAppendedToAnArrayBecomesANewElement and not a rule. dash has no
// arrays.
//
// The joining answer goes through appendArrayElem at the base, which is the
// same operation `a[0]+=x` is — measured, that is exactly where the value
// lands, at the *base* and not at the lowest subscript standing:
// `a=([5]=q); a+=x` is `declare -a a=([0]="x" [5]="q")` in bash, so an array
// with no first element grows one and `q` does not move. Reusing that path is
// also what makes the name's attributes fold once rather than twice: the join
// is appendedValue's, so `typeset -i` adds instead of concatenating, exactly
// as it does for the subscripted spelling.
//
// The adding answer lands one past the highest subscript, which is where
// assignArrayLiteral puts an appended literal's first word — one rule about
// where the end of an array is, not two.
//
// Reached only from assign's scalar branch, and deliberately not from
// storeArray: a rule at the store would also have to decide for `a=x`, which
// is the *other* disagreement (#1390) and lands differently — bash and ksh93
// write the first element and leave the rest, zsh replaces the array with a
// scalar. What is being decided here is what the append *operator* means over
// an array, which is the same boundary #1502's fix drew from the other side.
func (r *Runner) appendScalarToArray(name string, a Array, value string) {
	addsAnElement := r.ask(r.sem().ScalarAppendedToAnArrayBecomesANewElement,
		"a scalar appended to an array becoming a new element")
	if r.unspecified {
		return
	}
	if !addsAnElement {
		// The subscript as written, for the complaint that names one — the
		// base is never below the first element, so nothing can reach it.
		r.appendArrayElem(name, r.arrayBase(), strconv.Itoa(r.arrayBase()), value)
		return
	}
	a[a.pastTheEnd()] = value
	r.storeArray(name, a)
}

// scalarOverCompound is what a scalar store does to a name that is already
// holding an array or a table, and reports whether it has taken the write.
//
// One of the two disagreements a compound has with a plain assignment, and the
// other one is appendScalarToArray's: `a+=x` asks where the value *joins* and
// `a=x` asks whether there is anything left to join. See
// Semantics.ScalarAssignedOverACompoundReplacesTheName for the panel.
//
// It sits at the store rather than at the assignment statement, which is the
// whole of #1645. The statement path deleted the array itself, so `a=(1 2);
// a=x` was right and every other way of setting a name was wrong: `for a in
// x y z` over an array name read the array back on every pass, and so did
// `read a`, `select`, `getopts` and `${a::=x}`. Naming the callers that mean
// to replace is a list that only grows; naming the one caller that does not —
// assignedAsTheCompoundView, the write that keeps `$a` answering for an array
// `a` — is a list that is finished.
//
// The element the value lands on is the compound's first, whether or not there
// is one there: the array base for an array, and the key `0` for a table.
// `a=([5]=q); a=x` grows a new first element in bash and leaves `q` at 5.
//
// Returning false is not "nothing happened" — it is "the scalar store carries
// on", which is also what the replacing answer wants after it has taken the
// compound away.
func (r *Runner) scalarOverCompound(name, value string, form assignForm) bool {
	if form == assignedAsTheCompoundView {
		return false
	}
	_, isArray := r.Arrays[name]
	_, isTable := r.AssocArrays[name]
	if !isArray && !isTable {
		return false
	}
	if r.ask(r.sem().ScalarAssignedOverACompoundReplacesTheName,
		"a scalar assigned over an array or a table replacing it") {
		delete(r.Arrays, name)
		delete(r.AssocArrays, name)
		return false
	}
	if r.unspecified {
		return true
	}
	if isTable {
		r.setAssocElem(name, "0", value)
		return true
	}
	// The subscript as written, for the complaint that names one — the base
	// is never below the first element, so nothing can reach it.
	r.setArrayElem(name, r.arrayBase(), strconv.Itoa(r.arrayBase()), value)
	return true
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

// spanOutcome is what resolving a range on the left of an assignment came to.
//
// Three answers rather than a bool, because the two failures must not be
// confused: a subscript that is no range at all has to fall through to the
// single-subscript reading beside it, and one that *is* a range and would not
// resolve has already been reported — falling through there would blame the
// same text twice, once as a pair and once as an expression.
type spanOutcome uint8

const (
	// spanNotARange is a subscript this dialect does not read as a pair, or
	// one whose two readings name the same element. The single subscript
	// answers it.
	spanNotARange spanOutcome = iota
	// spanReported is a pair whose ends would not resolve. Already said.
	spanReported
	// spanResolved is a pair with both ends in hand.
	spanResolved
)

// assignSpan resolves the span a range on the left of an assignment names,
// where the dialect reads the comma as the separator of a range rather than as
// the arithmetic operator whose value is its right operand.
//
// The order is unsetSubscriptRange's, which is the same question about the
// same construct: a pair whose ends come to the same subscript is that
// subscript under either reading, so it is answered by the single subscript
// and no axis is asked — the discipline that keeps `a[2,2]=(x y)` from needing
// a column. Only then is the axis put, and only then is a failed end blamed,
// so a dialect without ranges reports the whole text as one expression exactly
// as it did before.
//
// The *operator* decides it too, and this is the row that would not have been
// guessed: `+=` reads no range at all. Measured on zsh 5.9.2 with `a=(1 2 3)`,
// `a[2,10]+=(x)` gives eleven elements — `[1][2][3]`, seven empties, `[x]` —
// which is the arithmetic comma's `10` padded to and appended at, and not the
// span 2 through 3 with something put after it. `a[2,3]+=x` gives `[1][2][3x]`
// for the same reason: it appends to element *3*.
func (r *Runner) assignSpan(a *syntax.Assign, text string) (from, to int, outcome spanOutcome) {
	return r.subscriptSpan(text, a.Append)
}

// subscriptSpan is assignSpan with the operator handed over rather than read
// off a parsed assignment, so that a builtin taking a subscripted *operand* —
// `read 'buf[2,3]'` — resolves the pair by the same rule an assignment does.
// One reading of a comma, reached two ways.
func (r *Runner) subscriptSpan(text string, appended bool) (from, to int, outcome spanOutcome) {
	if appended {
		return 0, 0, spanNotARange
	}
	lo, hi, ok := splitSubscriptPair(text)
	if !ok {
		return 0, 0, spanNotARange
	}
	from, errLo := r.subscriptValue(lo)
	to, errHi := r.subscriptValue(hi)
	if errLo == nil && errHi == nil && from == to {
		return 0, 0, spanNotARange
	}
	if !r.ask(r.sem().SubscriptCommaIsARange, "`${a[1,3]}` naming a range rather than one subscript") {
		return 0, 0, spanNotARange
	}
	if errLo != nil {
		r.fatal("%s\n", r.subscriptFailure(lo, errLo))
		return 0, 0, spanReported
	}
	if errHi != nil {
		// Measured: `a=(1 2 3); a[2,3/0]=(x y)` is `division by zero` and
		// ends the script, the same complaint the identical text inside
		// `$(( ))` makes and the same one a single bad subscript makes.
		r.fatal("%s\n", r.subscriptFailure(hi, errHi))
		return 0, 0, spanReported
	}
	return from, to, spanResolved
}

// spliceElementSpan replaces the elements a span names with words, which is
// what both spellings of a range assignment do — `a[lo,hi]=(p q)` and
// `a[lo,hi]=v` differ only in how many words there are. The length changes by
// the words' count less the span's, so the array grows, shrinks or holds.
//
// Measured on zsh 5.9.2, the one panel member that reads a range here, with
// `a=(1 2 3)` and fields compared rather than counted:
//
//	a[2,3]=(x y)      [1][x][y]        an equal count replaces, the length holds
//	a[2,3]=(x)        [1][x]           fewer shrinks
//	a[2,3]=(x y z)    [1][x][y][z]     more grows
//	a[2,3]=()         [1]              none deletes the span
//	a[2,3]=x          [1][x]           a scalar value is one word
//	a[2,10]=(x y)     [1][x][y]        an end past the last is the last
//	a[2,-1]=(x)       [1][x]           and a negative end counts back from it
//	a[0,2]=(x y)      [x][y][3]        a start below the first is the first
//	a[3,2]=(x)        [1][2][x][3]     an end before the start is an empty span,
//	                                   so the words go in and nothing comes out
//	a[5,6]=(x)        [1][2][3][][x]   a start past the last pads, then places
//	a[0,0]=(x)        refused          the span is wholly below the first
//
// The last three are the rows a symmetry argument gets wrong. The reversed
// range *inserts* rather than doing nothing, which is the same answer
// unsetElementSpan gives it — one construct, one rule. The padding is what the
// single subscript already does for `a[5]=(x)`. And the refusal is
// spanIsBelowTheFirstElement, which is why `a[0,0]` is refused where `a[0,1]`
// is not: one names a span entirely out of reach, the other one that begins
// out of reach and ends inside.
//
// It grew the array before rather than replacing the span. The arithmetic
// comma's reading took the pair for its right operand, so `a[2,3]=(x y)` ran
// as `a[3]=(x y)` and gave `[1][2][x][y]` — four elements where three were
// right, the old element 2 still standing in front of the new ones, at status
// 0 and with nothing said. A script that then indexed what it believed it had
// replaced is how it surfaced.
func (r *Runner) spliceElementSpan(name, text string, elems []string, from, to int, words []string) {
	if r.spanIsBelowTheFirstElement(from, to) {
		r.fatal("%s\n", Wording(r.diag().BadArraySubscript,
			"%[1]s[%[2]s]: bad array subscript", name, text))
		return
	}
	first, tail, within := r.spanOver(len(elems), from, to)
	if !within {
		// The span begins past the last element, so there is nothing to take
		// out: the gap becomes empty elements and the words follow them.
		out := make([]string, 0, first+len(words))
		out = append(out, elems...)
		for len(out) < first {
			out = append(out, "")
		}
		out = append(out, words...)
		r.setArray(name, out)
		return
	}
	out := make([]string, 0, first+len(words)+len(elems)-tail)
	out = append(out, elems[:first]...)
	out = append(out, words...)
	out = append(out, elems[tail:]...)
	r.setArray(name, out)
}

// storeThroughOperand writes a value through a name that may carry a
// subscript, which is the shape a *builtin* is handed one in: `read 'buf[2]'`
// and `read m[k]` arrive as a single word rather than as a parsed assignment,
// and the word has to be split and read before anything can be stored.
//
// Every caller that has a name and a value and no parsed assignment comes
// here: `read`, a registered builtin through [Runner.StoreThroughOperand] —
// which is how `sysread 'buf[$#buf+1]'` appends to a string — and the
// descriptor a redirection leaves behind, through setFdVar. Three callers
// and one walk of the brackets, because a second walk is how one of them
// would come to answer `buf[$#buf+1]` differently from the assignment
// `buf[$#buf+1]=x`.
//
// It is the assignment statement's own dispatch, reached from the other side
// and deliberately not restated: a declared table takes the text as a key, a
// pair takes the span, and a single subscript reaches setArrayElem — which is
// where the string dialect's character splice lives, so a builtin gets it by
// arriving here rather than by knowing about it.
//
// `read` did not arrive at all. It resolved its operand with setVar and never
// looked at the brackets, so `read 'buf[$#buf+1]'` created a parameter *named*
// `buf[3]` and left `buf` exactly as it was — nothing assigned, nothing said,
// status 0. Measured 2026-09-10, the whole panel fills the element for the
// array spelling: `a=(x y z); read 'a[2]'` on `Q` is `x Q z` in zsh 5.9.2,
// bash 5.3, bash 3.2 and ksh93 alike, so this is not the string dialect's
// question — it is one every shell answers and this one did not.
//
// A subscript that will not evaluate ends the script, which is the same
// complaint the identical text makes on the left of an assignment: measured,
// `read 'v[1/0]'` is `division by zero` at status 1.
func (r *Runner) storeThroughOperand(name, value string) {
	base, sub, ok := r.subscriptOperand(name)
	if !ok || !isPlainName(base) {
		r.setVar(name, value)
		return
	}
	// The *store* is speaking from here on, not the builtin that reached it,
	// and the location says so: measured, `read 'a[1/0]'` is `zsh:1: division
	// by zero` and `read 'v[0]'` is `zsh:1: v: assignment to invalid subscript
	// range` — the same two sentences, in the same place, as the bare
	// assignments `a[1/0]=x` and `v[0]=x`. Naming `read` in front of them
	// would report a builtin for a complaint the language makes.
	outer := r.inBuiltin
	r.inBuiltin = ""
	defer func() { r.inBuiltin = outer }()
	if r.assocDeclared(base) {
		r.setAssocElem(base, sub, value)
		return
	}
	from, to, outcome := r.subscriptSpan(sub, false)
	switch {
	case outcome == spanReported:
		return
	case outcome == spanResolved && r.spanReplacesElements(base):
		elems, _ := r.arrayElemsOfTheName(base)
		r.spliceElementSpan(base, sub, elems, from, to, []string{value})
		return
	case outcome == spanResolved && r.subscriptSplicesCharacters(base):
		if r.spanIsBelowTheFirstElement(from, to) {
			r.fatal("%s\n", Wording(r.diag().BadArraySubscript,
				"%[1]s[%[2]s]: bad array subscript", base, sub))
			return
		}
		r.spliceCharacterSpan(base, from, to, value, false)
		return
	}
	idx, err := r.subscriptValue(sub)
	if err != nil {
		r.fatal("%s\n", r.subscriptFailure(sub, err))
		return
	}
	r.setArrayElem(base, idx, sub, value)
}

// spanReplacesElements reports whether a range on the left of a *scalar*
// assignment reaches elements, which is what decides whether the value goes in
// as the one word that replaces the span.
//
// An array does, and so does a name holding nothing at all — that becomes one,
// so `unset a; a[2,3]=x` is `[][x]`.
//
// A declared table does not, and never arrives here anyway: the shell that
// reads ranges takes the comma inside a table's subscript for part of the key
// rather than for a separator, and the keyed branch ahead of this one already
// does that. Measured — `typeset -A h; h=(k v); h[a,b]=x` stores under the
// three characters `a,b` and leaves `k` alone.
//
// A plain string does not either, and that is the point of asking: the range
// reading of a string is a span of *characters* — measured, `s=hello;
// s[2,3]=x` is `hxlo` and `s[2,4]=QQ` is `hQQo` — so the answer here is no
// and the caller's next question, subscriptSplicesCharacters, is what sends
// it to spliceCharacterSpan. The two together are one boundary asked from
// both sides, and neither is a default.
func (r *Runner) spanReplacesElements(name string) bool {
	if _, isArray := r.Arrays[name]; isArray {
		return true
	}
	if r.assocDeclared(name) {
		return false
	}
	_, held := r.getVar(name)
	return !held
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
	return r.bareArrayReading(r.readArray(a), base, assigned)
}

// producedBareName is the same reading for an array that is generated rather
// than stored — `$FUNCNAME`, `$funcstack` — which had no reading at all
// before #1600 and answered as though the name were unset.
//
// It goes through bareArrayReading rather than repeating the axis, because
// the question is not a different one for a produced array: what a bare name
// gives is the dialect's answer about arrays, and where the producer's
// elements came from is not part of it.
func (r *Runner) producedBareName(elems []string) (string, bool) {
	// The same agreement the stored reading takes its shortcut on.
	if len(elems) == 1 {
		return elems[0], true
	}
	base, assigned := "", len(elems) > 0
	if assigned {
		base = elems[0]
	}
	return r.bareArrayReading(elems, base, assigned)
}

// bareArrayReading is the axis itself, asked once for both kinds of array.
//
// It answers set-ness as well as the value, and the two travel together
// because the shells disagree about both at once and in the same direction.
// Measured with no elements to read: zsh answers `${+funcstack}` 1 and
// `${funcstack+SET}` SET at a top level where `$#funcstack` is 0, and bash
// answers `${FUNCNAME+SET}` empty and `[[ -v FUNCNAME ]]` false outside a
// call. The shell that reads the whole list has a value — the empty join —
// so the name is set; the shell that reads the base element has no element
// zero, so it is not.
//
// A second function computing set-ness beside this one is the thing to avoid
// here: it would be the one place a produced array could come to a different
// answer from a stored one, which is exactly the defect being fixed.
func (r *Runner) bareArrayReading(elems []string, base string, assigned bool) (string, bool) {
	if r.ask(r.sem().ArrayScalarIsTheWholeArray, "a plain `$a` giving the whole array") {
		// Joined with the first character of IFS, exactly as `$*` is: an
		// empty array joins to the empty string, and the name is set.
		return strings.Join(elems, ifsFirst(r.ifs())), true
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
	// Asked first because it is the answer nearly every time, and because
	// the two questions below — which subscripts, in what order, and is
	// anything missing between them — are both already settled for an array
	// that has no gaps and starts at zero. See denseElems.
	if elems, dense := a.denseElems(); dense {
		return elems
	}
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
	if r.refusesEmptyParamSubscript(e) {
		// Refused, and every reading below is about a subscript there is
		// none of. Handled rather than absent, so no caller falls through to
		// the plain name.
		return nil, true
	}
	if len(e.Leading) > 0 {
		// A chain reads what the subscript before it named rather than what
		// the name holds, so none of the name's readings below apply: an
		// association's key is one link back, and this link is handed the
		// value that key found.
		src, ok := r.chainedSubscriptSource(e)
		if !ok {
			return nil, true
		}
		return r.subscriptAgainst(e, src)
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
	if r.assocDeclared(e.Name) {
		// The attribute decides the subscript's reading before anything is
		// looked up: a declared name takes it as a key, an undeclared one
		// falls through to the numeric path below. Asked of the name rather
		// than of a table produced to answer it — whether *any* table is
		// needed is the next question and not this one.
		return r.assocSubscriptOfTheName(e), true
	}
	elems, scalar, ok := r.subscriptTarget(e)
	if !ok {
		// The name holds nothing, and the subscript is still read: measured
		// 2026-09-11 on zsh 5.9.2, `w=; ${nosucharr[$w]}` is the same `bad
		// math expression: empty string` the declared array earns, so the
		// refusal is not something the name's absence excuses. Read here
		// rather than above, so the text is expanded once on this path and
		// once on the other.
		r.refusesEmptySubscriptText(r.subscriptText(e.Subscript()))
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
		// Copied, because the caller is allowed to write into what it gets
		// back and this is the runner's own storage. Every other source here
		// already hands out fresh storage — a named array through readArray,
		// an association through keys/values, a scalar through a one-element
		// literal — and the *unsubscripted* positional parameters through
		// namedBase, which writes this same copy. The subscripted spelling
		// was the one that aliased.
		//
		// A subscript is not a read-only path to the values: a range comes
		// back as `units[first : last+1]`, a subslice over the same backing
		// array, and the expansion-flag pass then applies rules 12 to 14 one
		// element at a time in place. So `${(q)@[1,-1]}` quoted the
		// parameters *themselves*, and the next read of `$@` quoted what the
		// previous read had already quoted — a level of backslashes added per
		// round trip rather than one taken away, which is #1622.
		return append([]string(nil), r.Params...), false, true
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

// subscriptNameIsAbsent reports whether the name a subscript was written on
// holds nothing at all: no array, no association, no value, and no special
// parameter behind it.
//
// It separates the two questions a quoted whole-array subscript with no
// elements used to answer with one: an array that exists and is empty, and a
// name that was never given anything. Only the second splits the panel — see
// Semantics.UnsetNameAtIsOneEmptyField, which is asked nowhere else.
//
// The association is asked separately because a declared one is not reached by
// subscriptTarget at all: arraySubscript answers it on an earlier branch, so a
// `typeset -A m` with no keys would look absent here and take the unset
// answer.
func (r *Runner) subscriptNameIsAbsent(e *syntax.ParamExpr) bool {
	if _, ok := r.assocFor(e.Name); ok {
		return false
	}
	_, _, ok := r.subscriptTarget(e)
	return !ok
}

// wholeSubscriptOnAScalar reports whether `[@]` or `[*]` was written on a name
// that is set and holds one string rather than a list.
//
// It is the shape both readings of that spelling turn on, and it is one
// function because the two readings kept drifting apart while it was two: the
// length grew the scalar reading in #1553 and the slice was still counting a
// list of one a day later (#1850). What differs between them is only *which*
// axis is then asked, so the shape is answered here and the question is asked
// at each site.
//
// A name that is absent or holds a list is outside it. An unset name is
// nothing under either reading, and a real array is a list in every column, so
// neither has two readings to choose between.
func (r *Runner) wholeSubscriptOnAScalar(e *syntax.ParamExpr) bool {
	return r.wholeArrayIndex(e) && !r.nameIsAList(e.Name) && !r.subscriptNameIsAbsent(e)
}

// wholeSubscriptSlicesAScalar reports whether `${s[@]:off:len}` on a name
// holding one string slices that string's characters rather than a list whose
// only element is the whole value.
//
// Declining sends the node down the scalar path, which is the point: the
// offsets then count exactly what they count for `${s:off:len}`, so the
// negative offset, the negative length and the locale's idea of a character
// all arrive already answered instead of being written out a second time
// beside the list slice. See Semantics.WholeSubscriptOnAScalarSlicesIt for
// the panel.
//
// The length is not this question — `${#s[@]}` splits the panel a different
// way and has an axis of its own — and an indirection is not either, since
// `${!s[@]}` yields subscripts before any offset is read.
func (r *Runner) wholeSubscriptSlicesAScalar(e *syntax.ParamExpr) bool {
	if e.Op != syntax.ParamSubstring || e.Length || e.Indirect {
		return false
	}
	if !r.wholeSubscriptOnAScalar(e) {
		return false
	}
	return r.ask(r.sem().WholeSubscriptOnAScalarSlicesIt,
		"`${s[@]:off:len}` on a scalar slicing the value it holds")
}

// splitSubscriptPair splits a subscript at its first top-level comma, whatever
// follows it.
//
// Nested parentheses and brackets hold their own commas, so `${a[f(1,2),3]}`
// has two halves and not three. A *third* half is the caller's question rather
// than this one's, because the two sides of an assignment answer it
// differently — see splitSubscriptRange for the reading that refuses one and
// assignSpan for the reading that does not.
func splitSubscriptPair(idx string) (lo, hi string, ok bool) {
	at, _ := topLevelComma(idx)
	if at < 0 {
		return "", "", false
	}
	return idx[:at], idx[at+1:], true
}

// splitSubscriptRange splits `1,3` into its two halves, reporting whether the
// subscript is written as a pair at all.
//
// One comma exactly, which is the *reading* side's rule: a subscript with two
// top-level commas is a pair in no shell measured there — `${a[1,2,3]}` is
// `bad substitution` in the one shell with ranges — so it is left to the
// arithmetic that reads it as an expression.
//
// The left of an assignment does not agree, and the asymmetry is measured
// rather than tidied away: `a=(1 2 3); a[1,2,3]=(x y)` gives `[x][y]` on zsh
// 5.9.2, which is the span 1 through the arithmetic `2,3` — so assignSpan
// splits on the first comma and lets the arithmetic have the rest.
func splitSubscriptRange(idx string) (lo, hi string, ok bool) {
	lo, hi, ok = splitSubscriptPair(idx)
	if !ok {
		return "", "", false
	}
	if at, _ := topLevelComma(hi); at >= 0 {
		return "", "", false
	}
	return lo, hi, true
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
	// The arithmetic alone, without the empty-text question: this is asking
	// which of two readings a subscript takes, and a subscript that will not
	// be read at all is the reporting caller's to refuse — once, rather than
	// once per probe that passed through here on the way.
	n, err := r.expressionValue(idx)
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
		return trimSubscript(w.Spans[0].Value)
	}
	return trimSubscript(strings.Join(r.expandWordNoSplit(w), ""))
}

// trimSubscript takes the blanks off a subscript, and leaves a subscript that
// is *nothing but* blanks alone.
//
// The exception is what separates two answers one shell gives in its own
// words: `${a[$w]}` with an empty `$w` is `bad math expression: empty string`
// there and `${a[ ]}` is `operand expected at end of string`, which is the
// expression reader complaining about two different positions. Trimming
// unconditionally spent that difference before anything could read it — see
// Semantics.EmptySubscriptTextIsAMathError.
//
// It is also what the panel does with a blank *key*: measured 2026-09-11 on
// bash 5.3.15, `declare -A m; m[" "]=v` and `${m[ ]}` name the same element,
// so the blanks are the key rather than space around one.
func trimSubscript(text string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	return strings.TrimSpace(text)
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
	if err := r.emptySubscriptText(text); err != nil {
		return 0, err
	}
	return r.expressionValue(text)
}

// emptySubscriptText is a subscript whose text came out empty or blank once
// it was expanded, where one shell in the panel will not read it as an
// expression at all — see Semantics.EmptySubscriptTextIsAMathError for the
// measurements and for the three neighbors this is not.
//
// Two sentences rather than one, because the shell that refuses gives two:
// nothing at all is `empty string` and blanks are the expression running out,
// which is the reader complaining about two different positions. The second
// is built as the parse failure it is, so it is worded by the same path
// `$(( a[ ] ))` is worded by rather than by a second copy of it.
func (r *Runner) emptySubscriptText(text string) error {
	if strings.TrimSpace(text) != "" {
		return nil
	}
	if !r.ask(r.sem().EmptySubscriptTextIsAMathError,
		"a subscript whose text expanded to nothing") {
		// Either the dialect reads it — the empty expression, which is
		// element zero — or no dialect was chosen and ask has said so. The
		// second is not a value this can invent, so the element answers as
		// it did.
		return nil
	}
	if text == "" {
		return arithError{
			msg:      Wording(r.diag().EmptySubscriptTextExpanded, "a subscript that expanded to nothing"),
			complete: true,
		}
	}
	return &syntax.Error{Kind: syntax.ErrArithOperandEnd, Expr: text, Token: text}
}

// refusesEmptySubscriptText reports a subscript that expanded to nothing where
// the dialect refuses one, and says whether it did.
//
// For the paths that have no element to look up afterwards and so never reach
// subscriptIndex, which would have reported it for them.
func (r *Runner) refusesEmptySubscriptText(text string) bool {
	err := r.emptySubscriptText(text)
	if err == nil {
		return false
	}
	r.diagf("%s\n", r.subscriptFailure(text, err))
	r.expandErr = true
	return true
}

// expressionValue is subscriptValue without the empty-text question: the
// arithmetic the two sites share.
//
// The substring range is what needs it. An offset that expanded to nothing is
// zero in every column — measured, `x=abcdef; w=; ${x:$w:2}` is `ab` even in
// the shell that refuses the same emptiness in a subscript — so the question
// is asked where a subscript is read and not here.
func (r *Runner) expressionValue(text string) (int, error) {
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
	if produce, ok := r.DynamicArrays[name]; ok {
		// The second half of #1600. This is what decides whether a bare name
		// is an array at all, so without it a produced one was not: `${#a}`
		// counted nothing, and the rewrite that turns a bare name into
		// `${a[@]}` or `${a[*]}` declined before it began.
		return len(produce(r)), true
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
	// Only a range asks what it ranged *over*; the three answers above are
	// the subscript's own and need no source at all, which is why the source
	// is read here rather than at the top.
	return r.subscriptSourceIsAList(e)
}

// subscriptSourceIsAList reports whether what a subscript reads is a list of
// elements rather than one string, which is the question a range's shape
// turns on.
//
// A name supplies it from what it holds. A chain supplies it from the
// subscript before it — `${a[1,3][1,2]}` ranges over the three elements the
// first range named, where `${a[1][1,2]}` ranges over the characters of the
// one element it named — and that is the *same* question asked of the
// previous link, so this recurses into subscriptYieldsAList rather than
// keeping a second reading of the rule beside it.
func (r *Runner) subscriptSourceIsAList(e *syntax.ParamExpr) bool {
	if n := len(e.Leading); n > 0 {
		return r.subscriptYieldsAList(r.chainLink(e, n-1))
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
	if e.Inner != nil && e.Index == nil {
		// A nested expansion with no subscript of its own joins for the same
		// reason: `@` is a spelling, and this one does not wear it. Measured
		// on zsh 5.9.2 with `a=("a b" c)` — `"${${a[@]}}"` is one field
		// holding `a b c`, `${${a[@]}}` unquoted is two, and the subscripted
		// spellings split the same way the name's do: `"${${a[@]}[@]}"` is
		// one field each and `"${${a[@]}[*]}"` is one joined.
		//
		// The `[@]` of the *inner* does not reach out here. It is what made
		// the inner a list; whether the outer keeps those fields in quotes is
		// the outer's own spelling, and reading the inner's would make
		// `"${${a[@]}}"` three fields where the shell gives one.
		return true
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

// arrayForWrite is the array an element assignment starts from: what the name
// already holds, produced or stored, as storage this caller may write into.
//
// A produced array has nothing in r.Arrays, so reading that table directly
// made `argv[2]=x` on a name whose elements come from a producer start at an
// empty array — the write landed at position 2 of nothing, and the elements
// the producer would have reported were gone. The elements are copied rather
// than aliased, because the producer's slice may be the runner's own storage
// and an element assignment writes in place.
func (r *Runner) arrayForWrite(name string) Array {
	if a, stored := r.Arrays[name]; stored {
		return a
	}
	if produce, produced := r.DynamicArrays[name]; produced {
		a := make(Array, 0)
		for i, v := range produce(r) {
			a[i] = v
		}
		return a
	}
	return Array{}
}

// refusesEmptyParamSubscript is `${a[]}` — brackets written with nothing at
// all between them — where the dialect refuses it, and reports whether it
// did.
//
// Five of the six columns refuse and each ends the input; ksh93 reads the
// brackets as an expression that happens to be empty, which is element zero,
// and that is what this engine already did in every dialect. So the wrong
// answer was the quiet kind: `${s[]}` on a scalar came back with the scalar
// at status 0 where the shell had stopped (#1763).
//
// **Asked of the node and before anything is expanded**, because a subscript
// that *arrived* empty is not this: `w=; ${m[$w]}` is `[]` at status 0 in the
// shell that refuses `${m[]}`, the subscript text being `$w` rather than
// nothing. That is the opposite of the arithmetic site, where the parameters
// go in before the expression is read and an empty `$w` really does produce
// `m[]` — see Semantics.EmptyArithSubscript, which is the same text one
// construct over with a different answer.
//
// Only the plain shape. A flag group selects rather than indexes and a chain
// reads what the link before it named, and neither construct exists in a
// column that refuses this, so neither has a measured answer to give.
func (r *Runner) refusesEmptyParamSubscript(e *syntax.ParamExpr) bool {
	if e.IndexFlags != nil || len(e.Leading) > 0 || !writtenEmptySubscript(e.Index) {
		return false
	}
	if !r.ask(r.sem().EmptyParamSubscriptIsAnError,
		"`${a[]}`, a subscript written with nothing in it") {
		// Either the dialect reads it — ksh93, where the empty expression is
		// element zero — or no dialect was chosen and ask has said so. The
		// second is not a value this can invent, so the expansion is still
		// handled and produces nothing.
		return r.unspecified
	}
	if w := r.diag().EmptyParamSubscript; w != "" {
		// A sentence of the dialect's own, which is zsh alone: the subscript
		// machinery's complaint, with no name in it and no bad-substitution
		// wording around it.
		r.diagf("%s\n", w)
		r.expandErr = true
		return true
	}
	// Everyone else gives their ordinary bad-substitution sentence, subject
	// and all — which is why this goes through the one place that words it
	// rather than spelling it again here.
	r.reportBadSubstitution(e)
	return true
}

// writtenEmptySubscript reports whether the brackets hold nothing at all,
// asked of the word as it was *written*.
//
// A word with no spans is the whole of it, and that is a fact about the
// parser worth stating because two near neighbors are not this:
//
//   - `${m[""]}` is a *key*, written as two characters, and it looks up the
//     empty one — measured on zsh 5.9.2, `typeset -A m; m[""]=4; ${m[""]}`
//     is `4` where `${m[]}` beside it is `invalid subscript`. It comes to one
//     span holding an empty literal, so a test on the literal's *text* would
//     have swallowed it.
//   - `${a[$w]}` and `${a[ ]}` come to a span each as well, whatever they
//     expand to. Whether an empty `$w` or a blank means anything is somebody
//     else's axis — see Semantics.BlankArithSubscriptIsTheEmptyExpression —
//     and not this one.
func writtenEmptySubscript(w *syntax.Word) bool {
	return w == nil || len(w.Spans) == 0
}

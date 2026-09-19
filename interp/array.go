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
type Array map[int]Element

// Element is what one subscript holds.
//
// A string in five of the six columns, and a struct here because of the
// sixth: ksh93 lets an element hold a value of its own. `a=(x y); a[1]=(p q)`
// leaves a two-element array whose second element *is* an array — `${a[1][1]}`
// reads `q` back there, `a[1]+=(r)` appends to it, and `typeset -p a` prints
// `typeset -a a=(x (p q) )`. None of that survives being flattened to a
// string, and the element's own scalar reading is not the flattening either:
// see Element.scalar.
//
// Exported because Runner.Arrays is, and a field rather than a bare string so
// that the store has one kind of element rather than two tables that can
// drift. Every dialect but ksh93 leaves Nested nil and pays a word for it.
type Element struct {
	// Str is the value where the element holds a string.
	Str string
	// Nested is the array the element holds instead, and nil where it holds a
	// string. An *empty* nested array is not nil and is not the empty string:
	// `a=(x y z); a[1]=()` leaves element 1 an array with nothing in it, which
	// still counts as an element and reads back as the two characters `(` and
	// `)` with a newline between them.
	Nested Array
}

// Scalar is an element holding a string, which is every element in five of the
// six columns.
//
// Exported because a dialect builds produced tables of them, an embedder hands
// whole arrays in, and Runner.Arrays is exported itself — a caller that can
// name the map can name what goes in it.
func Scalar(v string) Element { return Element{Str: v} }

// scalar is what the element reads as where one string is wanted — `${a[1]}`,
// a field of `"${a[@]}"`, the subject of `${#a[1]}`.
//
// Measured on ksh93u+ 2012-08-01, which is the only column that can have a
// nested element at all:
//
//	a=(x y); a[1]=(p q);    ${a[1]} -> p        the nested array's own first
//	a=(x y z); a[1]=();     ${a[1]} -> a newline between two parens
//
// The empty rendering is not an economy: `printf "[%s]" "${a[@]}"` there
// prints `[(`, a newline, `)]`, so an element holding an empty array is one
// field and that field has a newline in it.
func (e Element) scalar() string {
	if e.Nested == nil {
		return e.Str
	}
	if len(e.Nested) == 0 {
		return "(\n)"
	}
	return e.Nested[0].scalar()
}

// equal reports whether two elements hold the same value.
//
// Its own method because an element may hold an array, which makes the type
// uncomparable: `==` does not compile over it and maps.Equal does not accept
// it. A pointer to the nested array would have compiled and been wrong — two
// arrays built the same way are the same element and would have compared
// different.
func (e Element) equal(f Element) bool {
	if e.Str != f.Str || (e.Nested == nil) != (f.Nested == nil) {
		return false
	}
	return e.Nested == nil || e.Nested.equal(f.Nested)
}

// clone is a copy a write may reach without the original seeing it, to any
// depth.
//
// maps.Clone is one level short of that now, and the level it is short by is
// the one a script can see: an element holding a nested array holds a *map*,
// which a shallow copy shares. Measured — `a=(x y); a[1]=(p q); ( a[1]=Z )`
// left `typeset -a a=(x (Z q) )` in the **parent**, because a string written
// through a subscript reaches the nested array's own first element and the
// subshell's table was pointing at the parent's.
//
// Every place that copied an array shallowly calls this instead, rather than
// the one that showed the bug: a subshell, a scope's shadow, a saved
// assignment prefix and a heredoc's snapshot are four spellings of the same
// promise, and a rule stated at one of them is a rule the other three
// contradict.
func (a Array) clone() Array {
	if a == nil {
		return nil
	}
	out := make(Array, len(a))
	for k, v := range a {
		out[k] = v.clone()
	}
	return out
}

// clone is the same for one element: free for the string every column but one
// stores, and a copy of the nested array otherwise.
func (e Element) clone() Element {
	if e.Nested == nil {
		return e
	}
	return Element{Str: e.Str, Nested: e.Nested.clone()}
}

// equal reports whether two arrays hold the same elements at the same
// subscripts, to any depth.
func (a Array) equal(b Array) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		w, held := b[k]
		if !held || !v.equal(w) {
			return false
		}
	}
	return true
}

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
		out[k] = v.scalar()
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
	name = r.throughNameref(name)
	a := make(Array, len(elems))
	for i, v := range elems {
		a[i] = Scalar(v)
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
	if r.arraysAreAView {
		// This shell has written to the name, so it is no longer only looking
		// at what it inherited — which is what one column's element unset
		// turns on. See Runner.unsetEmptiesAnUnwrittenArray.
		if r.subshellWroteArrays == nil {
			r.subshellWroteArrays = map[string]bool{}
		}
		r.subshellWroteArrays[name] = true
	}
	// A compound variable is not a thing an array write shares a name with:
	// the elements replace the whole tree. See compoundVariableRetyped.
	r.compoundVariableRetyped(name)
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
	// Written to, so the name leaves the declared-only set whatever it was
	// holding before. The one chokepoint every indexed write reaches, which
	// is what this note relies on — see compounddeclaredonly.go.
	r.compoundWasAssigned(name)
	if len(a) > 0 {
		// And the third state, which only grows: an array that has held an
		// element and lost every one of them is not an array that never held
		// any, and one listing tells them apart. Set rather than written on
		// every store, because the emptying write comes through here too and
		// would clear exactly what it is supposed to record.
		setBool(&r.compoundHeldAnElement, name, true)
	}
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
		// Already folded, a few lines above, and folding it again would
		// evaluate the same text twice — which shows as a duplicated
		// complaint when the fold is an integer attribute that failed. See
		// Runner.viewIsAlreadyFolded.
		r.viewIsAlreadyFolded = true
		r.setVarAs(name, a[lo].scalar(), assignedAsTheCompoundView)
		r.viewIsAlreadyFolded = false
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
		out[pos] = Scalar(e)
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
	// The letter retypes a compound variable as surely as a literal does:
	// `c=(a=1); typeset -a c` lists `typeset -a c` with no members left.
	r.compoundVariableRetyped(name)
	r.Arrays[name] = Array{}
	// Declared and not assigned — see compounddeclaredonly.go.
	r.compoundDeclaredOnly(name)
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
	name = r.throughNameref(name)
	// An element write is a `.set` event with a subscript on it — measured,
	// `a=(x y); a[1]=z` enters `a.set` with `${.sh.name}` as `a` and
	// `${.sh.subscript}` as `1`, and a hook that rewrites `${.sh.value}` is
	// what the element ends up holding. The mark stays on for the store
	// because that store keeps the whole name's scalar view in step through
	// setVarAs, where the same event would otherwise fire a second time with
	// no subscript at all. See interp/discipline.go.
	if v, ran := r.disciplineWrite(name, disciplineSet, sub, value); ran {
		value = v
		defer r.suppressDiscipline(name, disciplineSet)()
	}
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
			r.storeArray(name, insertAtTheFront(a, Scalar(value)))
			return
		}
		if r.unspecified {
			return
		}
		r.failedSubscript("%s\n", Wording(r.diag().BadArraySubscript,
			"%[1]s[%[2]s]: bad array subscript", name, sub))
		return
	}
	// What the name's attributes make of the value, **before** it reaches the
	// array — the same fold a keyed write already does in setAssocElem and
	// the same axis it asks.
	//
	// The order is the whole of it. storeArray folds the elements it is
	// handed, so a value folded *there* is read back out of the cell it has
	// already replaced: with `typeset -i a`, `a[0]=2` then `a[0]=a[0]+4`
	// evaluated `a[0]+4` after `a[0]` held the text `a[0]+4`, which is an
	// expression that reads itself. The element read has no depth bound
	// (arithElemValue), so what came back was not a wrong answer but a **Go
	// stack overflow that killed the shell** — measured on origin/main
	// 5083d624d in `cmd/bash` and `cmd/ksh` alike, where bash 5.3.20 and
	// ksh93u+ both answer 6.
	//
	// The keyed spelling of the same three lines was right throughout, which
	// is what says this is the order and not the fold: `typeset -Ai m;
	// m[k]=2; m[k]=m[k]+4` is 6 here and there. One helper carried the fix
	// and the other did not.
	folded, ok := r.elementValueFolded(name, value)
	if !ok {
		// The integer evaluation failed and has already said so, or the
		// dialect answered nothing. Either way the array is left as it
		// stands rather than taking a value nobody computed.
		return
	}
	a[pos] = stringWritten(a[pos], folded)
	r.storeArray(name, a)
}

// elementValueFolded is compoundElemsFolded for the one value an element write
// is carrying, asked before the value is placed rather than after.
//
// The same three questions in the same order — does the name carry a folding
// attribute, would the fold change this value, and does the dialect send an
// element through it — so the two callers cannot answer differently.
//
// The **width** letters are among the attributes asked about, and the axis
// answers them with nothing added: measured 2026-09-18, `typeset -a n;
// typeset -L 3 n; n=(abcd efgh)` is `(abc efg)` on ksh93u+ and
// `typeset -aL3 n=( abcd efgh )` on zsh 5.9.2, which is the same split the
// case letters make and in the same direction. What the
// bool says is "the value below may be stored": false where the evaluation
// failed and has already reported, and where the axis went unanswered.
func (r *Runner) elementValueFolded(name, value string) (string, bool) {
	_, width := r.fieldWidth[name]
	if !r.integer[name] && !r.lowered[name] && !r.uppered[name] && !width {
		return value, true
	}
	if !r.attributeWouldChange(name, value) {
		return value, true
	}
	if !r.ask(r.sem().CompoundElementsGoThroughTheAttribute,
		"an element written to an attributed name going through the attribute") {
		return value, !r.unspecified
	}
	return r.attributeFolded(name, value)
}

// stringWritten is what a *string* write leaves in an element, which is not
// always a string: where the element holds a nested array the write reaches
// that array's own first element and the nesting stays.
//
// Measured on ksh93u+ 2012-08-01, 2026-09-13, which is the one column whose
// elements can nest — and each row here would have been a guess:
//
//	a=(x y); a[1]=(p q); a[1]=z    typeset -a a=(x (z q) )
//	a=(x y); a[1]=(p q); a[1]+=z   typeset -a a=(x (pz q) )
//	a=(x y); a[1]=();    a[1]=z    typeset -a a=(x z)
//
// So the write does not flatten a nested array and does not sit beside it; it
// goes *in*. The third row is the exception that states the rule: an empty
// nested array has no first element for the write to reach, and the element
// goes back to being a string rather than growing one.
//
// The append spelling arrives here already joined — appendArrayElem reads the
// element's scalar and hands the result over — so one rule covers both.
func stringWritten(e Element, value string) Element {
	if len(e.Nested) == 0 {
		return Scalar(value)
	}
	e.Nested[0] = stringWritten(e.Nested[0], value)
	return e
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
func insertAtTheFront(a Array, e Element) Array {
	out := make(Array, len(a)+1)
	for _, pos := range a.subscripts() {
		out[pos+1] = a[pos]
	}
	out[0] = e
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
		v, ok := r.appendedValue(name, a[pos].scalar(), value)
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
	a[a.pastTheEnd()] = Scalar(value)
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
		if _, produced := r.DynamicArrays[name]; produced {
			// A **produced** array has no stored element for this to take
			// away: the producer answers ahead of anything here, so a removal
			// would be accepted and then read back as whatever the producer
			// says. The same argument storeArray's produced branch makes
			// about a write, and the measured answer where it can be asked —
			// 2026-09-18, bash 5.3.20's `unset 'DIRSTACK[2]'` is silent at 0
			// with the stack whole, where this reached the scalar path and
			// refused the name as not an array.
			//
			// Silent and 0, which is what the name *not* being a scalar
			// means: the refusal below is about a name holding a value that
			// has no elements, and a produced array has elements.
			return 0
		}
		return r.unsetScalarElem(name, idx, sub)
	}
	if r.unsetEmptiesAnUnwrittenArray(name) {
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
		a[pos] = Element{}
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
		// A start that will not evaluate is reported and *nothing is done*,
		// which is the single subscript's rule and the opposite of the end's
		// below. Measured 2026-09-12 on zsh 5.9.2, the one shell with ranges:
		// `unset "a[x+,2]"` on `(x y z)` complains at 1 and leaves three
		// elements.
		return true, r.badSubscriptToUnset(lo, errLo)
	}
	// A *end* that will not evaluate is reported and then carries 0 forward,
	// and the range is acted on with it. Measured the same day on the same
	// shell, and it is the row that would not have been guessed:
	//
	//	unset "a[1,x+]"   complains at 1 and leaves ["", x, y, z]
	//	unset "a[2,x+]"   complains at 1 and leaves [x, "", y, z]
	//	unset "a[-1,x+]"  complains at 1 and leaves [x, y, "", z]
	//	unset "a[4,x+]"   complains at 1 and leaves the three alone
	//
	// Every one of those is what the same range with a written 0 does, so
	// there is no second rule here: `[1,0]` is a reversed range, a reversed
	// range leaves an empty element where it would have begun, and the
	// fourth element in the first row is that. The whole of the divergence
	// was reporting and then returning (#1001).
	//
	// Not an axis. An axis records a *disagreement* between shells over the
	// same syntax, and only the shell that reads a comma as a range can be
	// asked this at all — the other four never reach here, since the reading
	// itself is already the question asked above. The asymmetry between the
	// two ends is one shell's and is written down rather than switched on.
	reported := 0
	if errHi != nil {
		reported = r.badSubscriptToUnset(hi, errHi)
		to = 0
	}
	// Anything further to complain about is swallowed once the endpoint has
	// been reported: measured, `unset "a[0,x+]"` writes the math error and
	// *not* the `invalid subscript range` a written `a[0,0]` also writes.
	// One failed subscript, one sentence.
	quietly := func(code int) (bool, int) {
		if reported != 0 {
			return true, reported
		}
		return true, code
	}
	if r.spanIsBelowTheFirstElement(from, to) {
		if reported != 0 {
			return true, reported
		}
		return true, r.refuseSubscriptToUnset(name, sub)
	}
	if a, isArray := r.Arrays[name]; isArray {
		return quietly(r.unsetElementSpan(name, a, from, to))
	}
	v, held := r.getVar(name)
	if !held {
		// Neither an element nor a character for any span to reach.
		return quietly(0)
	}
	if r.scalarUnsetReadsAsCharacters(v, sub) {
		return quietly(r.unsetCharacterSpan(name, v, from, to))
	}
	// The element reading, where a scalar is the one element at the base: a
	// span that reaches it takes the whole name away, exactly as the single
	// subscript naming it does, and one that does not is the question every
	// other subscript on a scalar asks. No shell measured reads both a range
	// and a scalar-as-element, so this is the two readings composed rather
	// than a column of its own.
	if first, tail, within := r.spanOver(1, from, to); within && tail > first {
		r.unsetName(name)
		return quietly(0)
	}
	if reported != 0 {
		return true, reported
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
			elems[pos] = v.scalar()
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
// **This is a cut and not a deletion**, which is the whole of what the string
// reading is and is not the array's rule with the element left off. What comes
// back is what lies in front of the start joined to what lies behind the end,
// each endpoint resolved and then clamped on its own — so when the end is
// before the start the two halves *overlap* and the string grows. Measured on
// zsh 5.9.2 with `v=hello`, 2026-09-12:
//
//	v[2,0]    hhello      "h" and "hello"
//	v[3,0]    hehello     "he" and "hello"
//	v[3,1]    heello      "he" and "ello"
//	v[4,2]    helllo      "hel" and "llo"
//	v[5,1]    hellello    "hell" and "ello"
//	v[9,0]    hellohello  a start past the last clamps to the whole string
//	v[9,3]    hellolo     and the end is still read where it is written
//	v[-1,1]   hellello    a negative start is counted back from the last
//	v[-2,-4]  helllo      and so is a negative end
//	v[2,-6]   hhello      an end before the first is the first
//
// `v[3,2]` — the reversed range one step deep — is the one that comes back
// unchanged, and it is the row the corpus had: "he" and "llo" reconstruct
// `hello`. That is why this looked like "a reversed range is invisible over a
// string" for as long as it did (#2373). It is not invisible; it is a cut
// whose halves happened to meet.
//
// So this does not go through spanOver, and the difference is deliberate: an
// array's span *replaces* what it names with one empty element, so a start
// past the last element has nothing to stand in front of and the array is left
// alone. A cut has no such case — a start past the last is the whole string,
// and joining it to a suffix is what `v[9,0]` shows.
func (r *Runner) unsetCharacterSpan(name, v string, from, to int) int {
	chars := r.units(v)
	first, tail := r.charSpanCut(len(chars), from, to)
	r.setVar(name, strings.Join(chars[:first], "")+strings.Join(chars[tail:], ""))
	return 0
}

// charSpanCut resolves a written range over n characters into the two points a
// cut joins: everything before first, and everything from tail on.
//
// Each endpoint takes the subscript rules — counted from the dialect's base,
// or back from the end when negative — and is then clamped to the string on
// its own. Clamping them *separately* is what leaves tail below first for a
// reversed range, which is the overlap unsetCharacterSpan's table records.
func (r *Runner) charSpanCut(n, from, to int) (first, tail int) {
	base := r.arrayBase()
	first = from - base
	if from < 0 {
		first = n + from
	}
	tail = to - base + 1
	if to < 0 {
		tail = n + to + 1
	}
	return clampToLength(first, n), clampToLength(tail, n)
}

// clampToLength brings a resolved character position inside [0, n].
func clampToLength(pos, n int) int {
	if pos < 0 {
		return 0
	}
	if pos > n {
		return n
	}
	return pos
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
	if a.IndexText != "" && !pairIsWritten(a.IndexText) {
		// The comma has to have been **written** to separate a pair, and the
		// source says it was not: measured on zsh 5.9.2, 2026-09-12,
		// `a=(p q r s); i="1,2"; a[$i]=Z` writes the *first element* there
		// and leaves the rest, where splitting the expanded text replaced
		// the span (#2160).
		return 0, 0, spanNotARange
	}
	return r.subscriptSpan(text, a.Append)
}

// pairIsWritten reports whether the source spelled a top-level comma in this
// subscript, which is what makes it a pair at all.
func pairIsWritten(written string) bool {
	at, _ := topLevelComma(written)
	return at >= 0
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

// assignWholeArraySubscript performs `a[@]=v`, which the panel answers five
// ways over two questions — see Semantics.WholeArraySubscriptAssigningAnArray
// and WholeArraySubscriptAssigningATable, where the rows are.
//
// Two fields rather than one because bash and zsh swap sides between them:
// bash refuses this over an array and stores a key for a table, and zsh
// writes every element of an array and refuses it over a table. A single
// field would have had to give one of them the other's answer.
//
// The kind of the name is read *before* anything is stored, since the taking
// answer replaces the name's contents and would make its own question moot.
//
// The caller asks wholeArraySubscript of the subscript **as written** rather
// than of what it expands to, and that is measured rather than an economy: in
// the shell that reads this spelling, `i=@; a[$i]=Z` is `bad math expression:
// operand expected at '@'` and `a["@"]=Z` is the same complaint about `"@"`.
// So a subscript that merely comes out `@` is an arithmetic subscript like any
// other, and only the typed characters name every element.
func (r *Runner) assignWholeArraySubscript(a *syntax.Assign) {
	policy := r.sem().WholeArraySubscriptAssigningAnArray
	what := "the whole-array subscript on the left of an assignment"
	if r.assocDeclared(a.Name) {
		policy = r.sem().WholeArraySubscriptAssigningATable
		what = "the whole-array subscript on the left of an assignment to a table"
	}
	switch policy {
	case WholeArraySubscriptNamesEveryElement:
		value := r.assignValue(a)
		if a.Append {
			// `a[@]+=Z` adds one element at the end rather than joining
			// every element: measured, `x=(p q); x[@]+=Z` is `p q Z`.
			elems, _ := r.arrayElemsOfTheName(a.Name)
			r.setArray(a.Name, append(elems, value))
			return
		}
		// The whole name and not its elements: a scalar and an unset name
		// both come out a one-element array, so this cannot read what is
		// there and splice into it.
		r.setArray(a.Name, []string{value})
	case WholeArraySubscriptIsAnOrdinaryKey:
		key, ok := r.assocAssignKey(a.Name, a.Index)
		if !ok {
			return
		}
		value := r.assignValue(a)
		if a.Append {
			v, joined := r.appendedValue(a.Name, r.AssocArrays[a.Name][key].scalar(), value)
			if !joined {
				return
			}
			value = v
		}
		r.setAssocElem(a.Name, key, value)
	case WholeArraySubscriptIsABadSubscript:
		// Reported, 1, and the rest of the command list given up — the same
		// cost a bare assignment to a frozen name has, and measured the same
		// way: the commands after it run when they are on the next line and
		// do not when they share this one.
		r.diagf("%s\n", Wording(r.diag().BadArraySubscript,
			"%[1]s[%[2]s]: bad array subscript", a.Name, a.IndexText))
		r.status, r.assignFailed = 1, true
		r.abandonTheCommand()
	case WholeArraySubscriptIsInvalidInAnAssignment:
		// The subscript is the verb here, not the name.
		r.fatal("%s\n", Wording(r.diag().InvalidSubscriptInAssignment,
			"%s: invalid subscript in assignment", a.IndexText))
	case WholeArraySubscriptIsASliceOfATable:
		r.fatal("%s\n", Wording(r.diag().SliceOfAnAssociativeArray,
			"%[1]s: attempt to set slice of associative array", a.Name))
	default:
		r.diagf("%s\n", r.unanswered(what))
		r.status, r.unspecified = 2, true
	}
}

// storeOperandWholeArraySubscript is `read 'r[@]'` and `printf -v 'r[*]'` —
// a builtin's output operand whose brackets name the **whole array** rather
// than an element — and reports the status the builtin carries along with
// whether the operand was answered here.
//
// No column reads those brackets as arithmetic and this walked to the
// evaluator in every dialect, so bash got `@: arithmetic syntax error:
// operand expected` where it writes `r[@]: bad array subscript`, and zsh
// ended the script over a line it fills at 0 (#3486, #3498). ksh93 was right
// by accident: it really does evaluate them, and `@` really is not an
// operand.
//
// **The table is asked first, and both halves are fields of their own**,
// because the assignment's pair does not answer for the operand: ksh93 swaps
// sides on the table — `m[@]=Z` is `@: invalid subscript in assignment` and
// ends the input where `read 'm[@]'` stores under the key `@` at 0 — and on
// the array it gives an arithmetic complaint the line survives where the
// assignment ends the input. bash parts from itself too: it gives the command
// list up for `x[@]=Z` and not for `read 'x[@]'`, measured — `read 'r[@]'
// <<< Y; echo "same=$?"` prints `same=1` with the array whole.
//
// That is four rows of six disagreeing, which is the same evidence that made
// WholeArraySubscriptAssigningATable a second field rather than a reading of
// WholeArraySubscriptAssigningAnArray. Reading the assignment's fields here
// was tried and gave the ksh column a refusal it does not make.
//
// The status is the refusal's 1 and not the builtin's own 2, which is the
// opposite of the bad-*name* refusal one branch over: measured, `printf -v
// 'r[@]' %s Q` is 1 where `printf -v '1x' %s Q` is 2. So this does not read
// Diagnostics.BuiltinBadNameStatusFor.
//
// One row is measured and not matched: bash answers 2 rather than 1 when the
// refusal stops a `read` with names still to come — `read 'r[@]' b` is 2 and
// `read 'r[@]'` is 1, with `b` untouched either way. That is
// Semantics.ReadRefusedWriteIsOneOnTheLastName's rule, which wants the count
// of names left, and the store is reached from `printf -v` as well and has
// no such count. Routing the refusal through readFrozenStatus instead would
// have given ksh93's `read 'r[1/0]' b` a 2 where it measures 1.
func (r *Runner) storeOperandWholeArraySubscript(base, sub, value string) (status int, refused, handled bool) {
	if r.assocDeclared(base) {
		switch r.sem().StoreOperandWholeArraySubscriptOverATable {
		case WholeArraySubscriptIsAnOrdinaryKey, WholeArraySubscriptNamesEveryElement:
			// A table has no elements to replace, so the column that takes
			// the spelling over an array answers the key here as well — and
			// the key is what bash and ksh93 both store.
			r.setAssocElem(base, sub, value)
			return 0, false, true
		case WholeArraySubscriptIsASliceOfATable:
			r.fatal("%s\n", Wording(r.diag().SliceOfAnAssociativeArray,
				"%[1]s: attempt to set slice of associative array", base))
			return r.status, true, true
		case WholeArraySubscriptIsInvalidInAnAssignment:
			r.fatal("%s\n", Wording(r.diag().InvalidSubscriptInAssignment,
				"%s: invalid subscript in assignment", sub))
			return r.status, true, true
		case WholeArraySubscriptIsABadSubscript:
			r.diagf("%s\n", Wording(r.diag().BadArraySubscript,
				"%[1]s[%[2]s]: bad array subscript", base, sub))
			return 1, true, true
		}
		r.diagf("%s\n", r.unanswered(
			"the whole-array subscript on a builtin's operand over a table"))
		r.status, r.unspecified = 2, true
		return 2, true, true
	}
	switch r.sem().StoreOperandWholeArraySubscript {
	case StoreOperandWholeArraySubscriptNamesEveryElement:
		// The whole name and not its elements, which is the same reading the
		// bare assignment takes there: `r=(1 2 3); read 'r[@]'` on `Y`
		// leaves one element holding `Y`. Not a split — measured, `read
		// 'r[@]' b` on `X Y` leaves `r` as the one element `X` and fills `b`
		// with `Y`, so the operand takes its field like any other name and
		// what makes the array one element is what the spelling means.
		r.setArray(base, []string{value})
		return 0, false, true
	case StoreOperandWholeArraySubscriptIsBad:
		// Reported, 1, nothing written, and the rest of the line still
		// running — which is how this parts from the unevaluable subscript
		// next door, where bash gives the command up. The sentence names the
		// operand as written and says nothing about arithmetic.
		r.diagf("%s\n", Wording(r.diag().BadArraySubscript,
			"%[1]s[%[2]s]: bad array subscript", base, sub))
		return 1, true, true
	case StoreOperandWholeArraySubscriptIsAnExpression:
		// `@` and `*` are not operands, so the evaluator's own complaint is
		// the answer: nothing is handled here and the caller walks on to the
		// arithmetic it would have reached anyway.
		return 0, false, false
	}
	r.diagf("%s\n", r.unanswered(
		"the whole-array subscript on a builtin's output operand"))
	r.status, r.unspecified = 2, true
	return 2, true, true
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
// A subscript that will not evaluate is an axis of its own, and it ended the
// script for everybody until #3485: only zsh does that. bash gives up the
// command it is running and carries on at the next one, and ksh93 leaves a
// failed builtin behind and runs the very next thing — so `read 'r[1/0]'`
// stopped a script that two of the three columns finish. See
// Semantics.BadSubscriptToAnOutputOperand for the rows.
func (r *Runner) storeThroughOperand(name, value string) (status int, refused bool) {
	base, sub, ok := r.subscriptOperand(name)
	if !ok || !isPlainName(base) {
		r.setOperandValue(name, value)
		return 0, false
	}
	// The *store* is speaking from here on, not the builtin that reached it,
	// and the location says so: measured, `read 'a[1/0]'` is `zsh:1: division
	// by zero` and `read 'v[0]'` is `zsh:1: v: assignment to invalid subscript
	// range` — the same two sentences, in the same place, as the bare
	// assignments `a[1/0]=x` and `v[0]=x`. Naming `read` in front of them
	// would report a builtin for a complaint the language makes.
	//
	// One column does name it — see Diagnostics.StoreOperandBadSubscript,
	// which is why the name is kept here rather than simply dropped.
	//
	// The *location* is a separate claim and one column keeps the builtin's
	// there — see Diagnostics.BadSubscriptKeepsTheBuiltinsLocation, measured
	// against a bare `$(( b c ))` in the same shell, which that column
	// locates the other way (#3496).
	builtin := r.inBuiltin
	r.inBuiltin = r.keptBuiltinLocation(builtin)
	defer func() { r.inBuiltin = builtin }()
	if st, refused := r.storeOperandEmptySubscript(base, name, sub, builtin); refused {
		// A subscript written with nothing in it, which is what `read
		// "a[$i]"` is once a blank `$i` has gone in. Ahead of the table's
		// key as well as of the arithmetic, which is measured — see
		// storeOperandEmptySubscript, where the rows are.
		return st, true
	}
	if wholeArraySubscript(sub) {
		// `read 'r[@]'` and `printf -v 'r[*]'` name the whole array rather
		// than an element, and no column sends the brackets to the
		// arithmetic evaluator — which is what this walked on to doing in
		// every dialect, so bash got the evaluator's sentence in place of its
		// own and zsh refused a line it fills (#3486, #3498). See
		// storeOperandWholeArraySubscript for the rows; the one column that
		// really does evaluate them hands the walk back rather than
		// answering, so its complaint stays the evaluator's own.
		if st, refused, handled := r.storeOperandWholeArraySubscript(base, sub, value); handled {
			return st, refused
		}
	}
	if r.assocDeclared(base) {
		r.setAssocElem(base, sub, value)
		return 0, false
	}
	from, to, outcome := r.subscriptSpan(sub, false)
	switch {
	case outcome == spanReported:
		return 0, false
	case outcome == spanResolved && r.spanReplacesElements(base):
		elems, _ := r.arrayElemsOfTheName(base)
		r.spliceElementSpan(base, sub, elems, from, to, []string{value})
		return 0, false
	case outcome == spanResolved && r.subscriptSplicesCharacters(base):
		if r.spanIsBelowTheFirstElement(from, to) {
			r.fatal("%s\n", Wording(r.diag().BadArraySubscript,
				"%[1]s[%[2]s]: bad array subscript", base, sub))
			return 0, false
		}
		r.spliceCharacterSpan(base, from, to, value, false)
		return 0, false
	}
	idx, err := r.subscriptValueOfReference(sub)
	if err != nil {
		// The one refusal in here a builtin has to fold into its own status:
		// the others end the script, so nothing reads what they left. See
		// Semantics.BadSubscriptToAnOutputOperand.
		return r.storeRefusalStatus(builtin, r.badSubscriptGivesUp(
			r.sem().BadSubscriptToAnOutputOperand,
			"how much a store through a builtin's operand gives up for an unevaluable subscript",
			Wording(r.diag().StoreOperandBadSubscript, "%[2]s",
				builtin, r.subscriptFailure(sub, err)))), true
	}
	r.setArrayElem(base, idx, sub, value)
	return 0, false
}

// setOperandValue is the store behind a builtin's plain output operand: the
// positional list where the operand is a position, and an ordinary parameter
// otherwise.
//
// One dialect takes an all-digit operand where a builtin wants a name —
// `read 1`, `getopts x 1` and `printf -v 1` alike, all three gated on
// Semantics.ReadNameOperands — and until this the store walked past it into
// setVar, so the value landed on a parameter *named* `1` that no expansion in
// that dialect reads back: status 0, nothing said, nothing stored. That is the
// same silence a subscripted operand had before #3555, one layer further in.
//
// The gate is ReadNameOperands read a fourth time, and it is the right one
// rather than a convenience: the three builtins that reach here judge their
// operand with it, so a dialect that refuses an all-digit operand never
// arrives with one and a dialect that takes it means the position.
//
// Measured 2026-09-18, zsh 5.9.2, each probe a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`:
//
//	set -- P Q; printf 'x\n' | read 5; echo "$# [$*]"    5 [P Q   x]
//	set -- P Q; getopts x 5 -x;         echo "$# [$*]"    5 [P Q   x]
//	set -- P Q; printf -v 5 %s ZZ;      echo "$# [$*]"    5 [P Q   ZZ]
//	set -- P Q R; printf 'x\n' | read 2; echo "$# [$*]"  3 [P x R]
//	set -- P Q; printf 'x\n' | read 007; echo "$# [$7]"  7 [x]
//
// So a position past the end **widens** the list to it and pads with empty
// words rather than writing nothing — which is the opposite of what the same
// operand does at a `{name}` redirection, where an out-of-range position
// writes nothing at all (see positionalFdVar). The two are measured apart and
// stay apart.
//
// Position 0 is `$0` in that shell and is deliberately not taken here: a write
// to it is visible inside the function that made it and gone when the call
// returns, which is a per-frame value this runner does not have — `$0` is
// derived from the call stack rather than stored. Measured the same day,
// `f() { printf 'x\n' | read 0; echo "[$0]"; }; f; echo "[$0]"` is `[x]` then
// the script's own name. Filed as #3672 rather than half-modeled here.
func (r *Runner) setOperandValue(name, value string) {
	if r.storeThroughPositional(name, value) {
		return
	}
	r.setVar(name, value)
}

// storeThroughPositional is the half of setOperandValue that knows the rule,
// and it reports whether it took the operand.
func (r *Runner) storeThroughPositional(name, value string) bool {
	if r.sem().ReadNameOperands != NamesAndPositionals {
		return false
	}
	n, ok := positionalFdVar(name)
	if !ok || n < 1 {
		return false
	}
	for len(r.Params) < n {
		r.Params = append(r.Params, "")
	}
	r.Params[n-1] = value
	return true
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
				r.storeArray(name, Array{0: {}})
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
	elem, assigned := a[0]
	base := elem.scalar()
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
	name = r.throughNameref(name)
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
			out = append(out, a[k].scalar())
		}
		return out
	}
	if r.ask(r.sem().ArraysAreSparse, "an unassigned subscript being no element at all") {
		out := make([]string, 0, len(subs))
		for _, k := range subs {
			out = append(out, a[k].scalar())
		}
		return out
	}
	from, to := a.extent(0)
	out := make([]string, 0, to-from+1)
	for i := from; i <= to; i++ {
		out = append(out, a[i].scalar())
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
	// A subscript is **arithmetic**, and arithmetic moves: `a[i++]` is a
	// different element every time it is read. One expansion asks for its
	// elements from more than one place — the source, the list path and the
	// length — so the answer is kept for the span and the subscript is
	// evaluated once, which is what both reference shells do. See sourceHold
	// in interp/expand.go, where the same rule is written down for the value
	// (#3104).
	if elems, ok, held := r.heldSubscript(e); held {
		return elems, ok
	}
	elems, ok := r.readArraySubscript(e)
	r.holdSubscript(e, elems, ok)
	return elems, ok
}

// readArraySubscript is arraySubscript with the hold off: the read itself.
func (r *Runner) readArraySubscript(e *syntax.ParamExpr) ([]string, bool) {
	if target := r.throughNamerefName(e.Name); target != e.Name {
		// A subscript written on a **name reference** indexes what the
		// reference points at: `v=(a b c); typeset -n r=v` reads `${r[1]}`
		// as `b` and writes `r[1]=Z` into `v`. Resolved once, here, on a
		// copy of the node — the readings below consult the name against
		// four tables and a special parameter, and resolving at each of them
		// is how one of them would be missed. The AST itself is never
		// touched: a `${r[1]}` inside a function body is parsed once and run
		// under a different reference every call.
		copied := *e
		copied.Name = target
		e = &copied
	}
	if r.refusesEmptyParamSubscript(e) {
		// Refused, and every reading below is about a subscript there is
		// none of. Handled rather than absent, so no caller falls through to
		// the plain name.
		return nil, true
	}
	if dotRanged(e) {
		// `${a[lo..hi]}`, which the grammar separated because the `..` was
		// written. See interp/dotrange.go.
		return r.dotRangeSubscript(e), true
	}
	if len(e.Leading) > 0 {
		if r.sem().ChainedSubscriptReadsANestedValue == Yes {
			// The other reading of the same text: the chain reaches *into*
			// the compound an element holds rather than counting through
			// what the link before it named. See interp/nestedchainsub.go.
			return r.nestedChainSubscript(e)
		}
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
		written := r.subscriptTextAsWritten(e.Subscript())
		r.refusesEmptySubscriptText(trimSubscript(written))
		r.absentNameSubscriptBeforeTheFirstElement(e, written)
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
	if e.IndexRange != nil {
		// A pair the parser separated, because the comma has to have been
		// *written* to separate one. Ahead of everything below, which reads
		// the subscript as one piece of text and would hand a group's
		// letters to the arithmetic.
		if r.subscriptIsReadAsItsIndex(e, src) &&
			r.ask(r.sem().SubscriptCommaIsARange, "`${a[1,3]}` naming a range rather than one subscript") {
			// A range names a span and `(k)` wants the one index a subscript
			// named, so there is nothing for it to answer: measured,
			// `${(k)x[1,2]}` is `invalid subscript` and the line ends at 1.
			return nil, r.reportIndexAndRange()
		}
		return r.flaggedRangeSubscript(e, src)
	}
	written := r.subscriptTextAsWritten(e.Subscript())
	idx := trimSubscript(written)
	if r.wholeArrayIndex(e) {
		if e.Length || e.Indirect {
			// `${#a[@]}` counts the elements and `${!a[@]}` names them;
			// neither reads a value, and measured, neither enters the hook
			// once. `${#a[1]}` is not this case and does fire — the length
			// of *one* element is a read of it — which is why the question
			// is asked on the whole-array branch alone.
			return elems, true
		}
		if !r.disciplineIsWatching(src.name, disciplineGet) {
			// Asked before the subscripts are worked out, and not only to
			// save the walk: naming an element counts from the array base,
			// and the base is an *axis*. A dialect-free runner reading
			// `${a[@]}` would have been asked to settle it for a hook that
			// does not exist.
			return elems, true
		}
		// One `.get` per element, each entered with its own subscript —
		// measured on ksh93u+ 2012-08-01, `${a[@]}` on three elements runs
		// the hook three times with `0`, `1`, `2`. See interp/discipline.go.
		return r.disciplinedElements(src.name, r.elementSubscripts(src.name, len(elems)), elems), true
	}
	if lo, hi, isRange := splitSubscriptRange(idx); isRange && !r.pairsAreSplitWhenWritten() {
		// A grammar whose parser does not separate a written pair, where the
		// only text there is to split is the expanded one. See
		// pairsAreSplitWhenWritten, which is the whole of the difference.
		if r.subscriptIsReadAsItsIndex(e, src) &&
			r.ask(r.sem().SubscriptCommaIsARange, "`${a[1,3]}` naming a range rather than one subscript") {
			return nil, r.reportIndexAndRange()
		}
		return r.rangeSubscript(src, idx, lo, hi)
	}
	// The *written* text, because a third comma is a fact about what was
	// typed: `${a[1,2,3]}` is a bad substitution in the shell with ranges,
	// where the same three numerals arriving through a parameter are read as
	// one expression that stops at the first comma (#2160).
	if at, extra := topLevelComma(r.writtenSubscript(e, idx)); at >= 0 && extra &&
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
	//
	// Read from the text as written, blanks and all, because a complaint
	// quotes it back and one column quotes the blanks with it (#2010). The
	// value is the same either way — the expression reader skips them.
	n, ok := r.subscriptIndexAsWritten(r.writtenSubscript(e, idx), written)
	if !ok {
		return nil, true
	}
	if r.subscriptIsReadAsItsIndex(e, src) {
		// The *index*, not the element — `(i)` answers where a value is and
		// never reads it, so there is no read for a discipline to be part of.
		return []string{itoa(r.forwardSubscriptIndex(n, len(elems)))}, true
	}
	if scalar {
		if v, ok := scalarElemAt(elems[0], n, r.arrayBase()); ok {
			return []string{v}, true
		}
		if n < 0 {
			// A string has one place and it is at the base, so counting back
			// from it reaches past the first element in the one step. The
			// same door elemAt goes through, with no end to count from,
			// which is what Semantics.SubscriptBeforeTheFirstElementNeedsAnElement
			// answers: measured, `a=x; echo "${a[-1]}"` is `a: bad array
			// subscript` in bash 5.3.20 and silent in ksh93u+.
			r.subscriptBeforeTheFirstElement(src.name, n, 0,
				subscriptLength{is: e.Length, written: r.writtenSubscript(e, idx)})
		}
		// No hook here, and that is measured rather than an omission: a
		// scalar has one place, so `g=raw; ${g[1]}` enters nothing in
		// ksh93u+ where `a=(p q r); ${a[9]}` enters the hook with `9`. The
		// one place it does have is read through varValue above, which has
		// already run the hook for it.
		return nil, true
	}
	v, held := r.elemAtFor(src.name, elems, n,
		subscriptLength{is: e.Length, written: r.writtenSubscript(e, idx)})
	if !r.disciplineIsWatching(src.name, disciplineGet) {
		// Before forwardSubscriptIndex, which counts a negative subscript
		// from the array *base* — an axis a runner with no dialect cannot
		// answer and must not be asked for a hook nobody defined.
		if held {
			return []string{v}, true
		}
		return nil, true
	}
	sub := itoa(r.forwardSubscriptIndex(n, len(elems)))
	if held {
		return []string{r.disciplinedElement(src.name, sub, v)}, true
	}
	// A subscript the array has no element at still fires: the element is a
	// place in an array and the array is there. A hook that says nothing
	// leaves the read absent, which is what keeps `${a[9]:-d}` taking the
	// default.
	if got, replaced := r.disciplineElementRead(src.name, sub); replaced {
		return []string{got}, true
	}
	return nil, true
}

// elementSubscripts is the subscript each of a name's elements answers to, in
// the order arrayElems hands the values back, for the hook that is entered
// once per element.
//
// The keys are asked of the stored array so that a sparse one says `0 3 9`
// rather than `0 1 2` — the same reading `${!a[@]}` gives, which is why it is
// arrayKeys and not a count. A produced array has no keys to ask and no gaps
// to have, so it counts from the base; so does any reading whose keys did not
// line up with its values, where counting is at least the right length.
func (r *Runner) elementSubscripts(name string, n int) []string {
	out := make([]string, n)
	if a, ok := r.Arrays[name]; ok {
		if keys := r.arrayKeys(a); len(keys) == n {
			for i, k := range keys {
				out[i] = itoa(k)
			}
			return out
		}
	}
	base := r.arrayBase()
	for i := range out {
		out[i] = itoa(base + i)
	}
	return out
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

// absentNameSubscriptBeforeTheFirstElement answers a negative subscript
// written on a name that holds nothing at all.
//
// The emptied array and the scalar reach the reach's own door already —
// Runner.elemAt and the scalar branch of Runner.subscriptOver — and this one
// did not, because subscriptTarget answers "the name holds nothing" before
// any element is counted and the subscript is then refused as text rather
// than resolved to a position. So the axis that decides it,
// Semantics.SubscriptBeforeTheFirstElementNeedsAnElement, was never asked on
// the one row it was most obviously about: measured 2026-09-18, `unset a;
// echo "[${a[-1]}]"; echo after` is `a: bad array subscript`, then `[]`, then
// `after` in bash 5.3.20 — the same three lines `a=()` and `a=x` give in that
// shell — and this engine was silent.
//
// **The length is not this route**, and that is measured rather than left
// out: `unset a; echo "[${#a[-1]}]"` is `[0]` and silent in the same shell,
// where the length over an *existing* name is a refusal of its own. A name
// that is not there has its length answered before the subscript is looked
// at, so the node's own Length is the guard.
//
// The text is the one the caller already expanded, because expanding the word
// twice would run a command substitution in it twice — the mistake #1915 was.
func (r *Runner) absentNameSubscriptBeforeTheFirstElement(e *syntax.ParamExpr, written string) {
	if e.Length || e.IndexRange != nil || r.wholeArrayIndex(e) {
		return
	}
	if r.sem().SubscriptBeforeTheFirstElementRead == SubscriptBeforeStartIsNothing {
		// The fast path the reach's own door takes, and here it also keeps a
		// dialect with no arrays from reading a subscript it has no reading
		// for at all.
		return
	}
	idx := trimSubscript(written)
	if lo, _, isRange := splitSubscriptRange(idx); isRange && lo != "" {
		return
	}
	n, ok := r.subscriptIndexAsWritten(r.writtenSubscript(e, idx), written)
	if !ok || n >= 0 {
		return
	}
	// No end to count back from, which is exactly what the axis is about.
	r.subscriptBeforeTheFirstElement(e.Name, n, 0, subscriptLength{})
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

// pairsAreSplitWhenWritten reports whether the parser separates a written pair
// into two ends, which is where the split belongs: **a comma has to have been
// written to separate one.** Measured on zsh 5.9.2, 2026-09-12,
// `i="1,2"; ${a[$i]}` is the *first* element there and not the range `1,2`.
//
// Asked of the grammar rather than of the semantics vector, because it is a
// fact about what the parser did with the source: where the grammar has
// subscript flag groups it has ranges too — no dialect measured has one
// without the other — and syntax.Parser.subscriptRange separates every pair
// it finds. A grammar without them never produced a SubscriptRange, so the
// run-time split over the expanded text is what it still gets, and for it
// that text and the written one are the same thing.
func (r *Runner) pairsAreSplitWhenWritten() bool {
	return r.dialect().ArraySubscriptFlags
}

// writtenSubscript is the subscript as the source spelled it, falling back to
// the text the caller has where the node carries none — a subscript on an
// expansion's result, or one this shell built at the run.
func (r *Runner) writtenSubscript(e *syntax.ParamExpr, expanded string) string {
	if e.IndexText != "" {
		return e.IndexText
	}
	return expanded
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
//
// The parser asks the same question of the subscript as *written*, so that a
// pair whose ends carry flag groups becomes two ends there rather than one
// operand here — see syntax.SubscriptRange. One implementation, because two
// would be two answers to one question and the pair would then be split in
// one place and not the other.
func topLevelComma(idx string) (at int, extra bool) {
	return syntax.SubscriptComma(idx)
}

// rangeSubscript answers a subscript written as a pair, `${a[1,3]}`.
//
// Both readings are worked out before either is chosen, and the axis is asked
// only when they disagree — `${a[2,2]}` is the second element whether the
// comma separates a range or joins two expressions, so it needs no answer, and
// neither does a pair either reading refuses.
func (r *Runner) rangeSubscript(src subscriptSource, idx, lo, hi string) ([]string, bool) {
	span, badEnd, spanErr := r.rangeElems(src.elems, src.scalar, lo, hi)
	whole, wholeErr := r.subscriptValue(idx)
	var one []string
	oneOK := wholeErr == nil
	if oneOK {
		if v, found := r.elemAt(src.name, src.elems, whole); found {
			one = []string{v}
		}
	}
	if spanErr == nil && oneOK && equalStrings(span, one) {
		return span, true
	}
	if r.ask(r.sem().SubscriptCommaIsARange, "`${a[1,3]}` naming a range rather than one subscript") {
		if spanErr != nil {
			// An end that will not evaluate is the *range* reading's
			// failure, and it was thrown away here: `${a[b c,2]}` came back
			// empty at status 0 where the shell with ranges writes `bad math
			// expression: operator expected at ``c''` and ends the line.
			// Reported against the end that failed, which is what names `c`
			// rather than the whole pair (#2161).
			r.diagf("%s\n", r.subscriptFailure(badEnd, spanErr))
			r.expandErr = true
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
//
// A start outside the array is not always *nothing*, and that half is
// measured rather than derived: an array answers some of those shapes with
// **one empty element**, which `${#a[lo,hi]}` counts as 1 and `${(@)a[lo,hi]}`
// makes one field of. Measured 2026-09-12 on zsh 5.9.2 over the whole grid of
// endpoints for arrays of five, two, one and no elements, the two blocks that
// leave one are:
//
//   - **above** the array, `first >= n`: one empty element when the
//     unclamped `last` is strictly greater than `first`. With five elements
//     `${#a[6,7]}` is 1 and `${#a[6,6]}` is 0, and `${#a[7,9]}` is 1 where
//     `${#a[9,9]}` is 0 — so it is the strictness and not the distance.
//   - **below** it, an `lo` written negative that counts back past the first
//     element: one empty element whenever `last` is at or above `first`,
//     equal ends included. `${#a[-8,-8]}` is 1 where the shape above at
//     equal ends is 0, so the two edges are not mirror images.
//
// The two are told apart by how `lo` was *written* rather than by where it
// landed: `-6` and `0` both normalize to the same position on a five-element
// array, and the negative answers 1 everywhere while the literal `0` answers
// exactly as `1` does.
//
// A *scalar* does neither — `s=hello; ${#s[6,7]}` and `${#s[-8,-8]}` are both
// 0 — so this is the element reading's alone.
//
// One cell of the grid is not reproduced, and it is recorded rather than
// fitted: on an array with **no** elements `${#a[0,0]}` is 1 there while
// `${#a[0,1]}` beside it is 0 and `${#a[0,2]}` is 1 again. A rule
// non-monotonic in `hi` is an artifact of the shell's own arithmetic rather
// than a statement about ranges, and writing it down here would be writing
// down that artifact.
//
// badEnd is the end that would not evaluate, named so that a caller reporting
// spanErr blames the half the shell blames rather than the whole pair.
func (r *Runner) rangeElems(elems []string, scalar bool, lo, hi string) (span []string, badEnd string, err error) {
	from, err := r.subscriptValue(lo)
	if err != nil {
		return nil, lo, err
	}
	to, err := r.subscriptValue(hi)
	if err != nil {
		return nil, hi, err
	}
	return r.rangeSpan(subscriptSource{elems: elems, scalar: scalar}, from, to), "", nil
}

// rangeSpan is rangeElems with both ends already evaluated, which is what a
// pair whose ends carry flag groups hands it: a search names an index and
// there is no text left to evaluate. One reading for both spellings, so the
// out-of-range rules above cannot come to hold for one and not the other.
func (r *Runner) rangeSpan(src subscriptSource, from, to int) []string {
	units, scalar := src.elems, src.scalar
	if scalar {
		units = r.units(src.elems[0])
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
			return outOfRangeSpan(last >= first)
		}
		first = 0
	}
	if first >= n && !scalar {
		return outOfRangeSpan(last > first)
	}
	if last >= n {
		last = n - 1
	}
	if last < first {
		return []string{}
	}
	span := units[first : last+1]
	if scalar {
		return []string{strings.Join(span, "")}
	}
	return span
}

// reportIndexAndRange refuses a subscript that is asked for the one index it
// named and names a span instead. Always handled, so the caller answers
// nothing rather than falling through to a reading the shell refuses.
func (r *Runner) reportIndexAndRange() bool {
	r.diagf("%s\n", Wording(r.diag().SubscriptIsAnIndexAndARange, "invalid subscript"))
	r.expandErr = true
	return true
}

// outOfRangeSpan is what a range whose start is outside the array comes to:
// one empty element, or nothing at all. See rangeElems for which is which and
// for the measurement behind it.
//
// A named function rather than a literal at each of the two sites, because
// the two differ only in the comparison and the answer they share is the
// surprising half.
func outOfRangeSpan(oneEmpty bool) []string {
	if oneEmpty {
		return []string{""}
	}
	return []string{}
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
	if e.IndexRange == nil {
		if r.pairsAreSplitWhenWritten() {
			// The parser had the chance and did not take it, so no pair was
			// written and the text is one subscript however many commas an
			// expansion put in it. Asking the expanded text instead made
			// `i="2,3"; ${a[$i,4]}` no range at all — three commas there —
			// and a quoted range that is not a range keeps its fields.
			return false
		}
		if _, _, ok := splitSubscriptRange(r.subscriptText(e.Subscript())); !ok {
			return false
		}
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
	return r.elemAtFor(name, elems, n, subscriptLength{})
}

// elemAtFor is that read with what a `${#a[-4]}` has to say about a subscript
// that reached past the first element. See subscriptLength, whose zero value
// is what every route but the parameter expansion's means.
func (r *Runner) elemAtFor(name string, elems []string, n int, length subscriptLength) (string, bool) {
	// When the sparse reading compacted a gap out of elems, a position can no
	// longer be counted there; the store still holds every position. Only a
	// *stored* array can be behind elems here — a produced one is read before
	// the table, in the same order arrayElems reads them.
	a, stored := r.Arrays[name]
	if _, produced := r.pipelineStatuses(name); produced {
		stored = false
	}
	compacted := stored && len(elems) != a.pastTheEnd()

	end := len(elems)
	if compacted {
		end = a.pastTheEnd()
	}
	var pos int
	if n < 0 {
		pos = end + n
	} else {
		pos = n - r.arrayBase()
	}
	if pos < 0 {
		if n < 0 {
			// Counting back past the first element, which is the one reach
			// that is not simply "no element". See
			// Runner.subscriptBeforeTheFirstElement for the three answers and
			// where each was measured; a non-negative subscript below the
			// base arrives here too and is the neighboring question, which
			// every column refuses alike.
			r.subscriptBeforeTheFirstElement(name, n, end, length)
		}
		return "", false
	}
	if compacted {
		v, ok := a[pos]
		return v.scalar(), ok
	}
	if pos >= len(elems) {
		return "", false
	}
	return elems[pos], true
}

// subscriptSubject is the subscript a refusal quotes back: the text as it was
// **written**, before any expansion, falling back to the text the caller has
// where the source is not available.
//
// One dialect names what was typed rather than what the text came to —
// measured 2026-09-07 and again 2026-09-12, `i=-9; a=(x); a[$i]=q` is
// `a[$i]: bad array subscript` in bash 5.3.15 and 3.2.57, and `a[$i]=(p q)`
// is `a[$i]: cannot assign list to array member` — and nothing the
// interpreter holds could say so, because a word that has been expanded no
// longer remembers its spelling. syntax.Assign.IndexText and
// syntax.ParamExpr.IndexText are that memory, on the same argument
// syntax.Redirect.Text is kept on.
//
// The fallback is not a nicety. A subscript that reached the store as a
// *string* — `typeset "a[$i]"=q`, `read "a[$i]"`, `unset "a[$i]"` — was
// expanded by the caller before any of it was source text, and the same
// column names the number there: `a[-9]`, measured. So those routes have no
// written text to offer and are right not to.
//
// It is the boundary refusals alone. An expression that will not *evaluate*
// is quoted back as the expanded text in every column — `i=1; a[$i/0]=x` is
// `1/0: division by 0` in bash — so subscriptFailure keeps what it was given
// (#1373).
func subscriptSubject(written, expanded string) string {
	if written != "" {
		return written
	}
	return expanded
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
	return trimSubscript(r.subscriptTextAsWritten(w))
}

// subscriptTextAsWritten is the same text before the blanks come off it.
//
// A caller that needs both takes this one and trims it itself, because
// expanding the word twice would run a command substitution in it twice — the
// mistake #1915 was. What needs the untrimmed text is the complaint about a
// subscript that would not evaluate: ksh93 quotes it back as written, so
// `${a[ 1/0 ]}` is ` 1/0 : divide by zero` there and the trimmed text could
// not say the blanks had been there (#2010). Nothing that *decides* anything
// reads it — a key, a range and the whole-array spellings are all settled on
// the trimmed text, which is what the panel matches.
func (r *Runner) subscriptTextAsWritten(w *syntax.Word) string {
	if w != nil && len(w.Spans) == 1 && w.Spans[0].Kind == syntax.Literal {
		return w.Spans[0].Value
	}
	return strings.Join(r.expandWordNoSplit(w), "")
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
	return r.subscriptValueAsWritten(text, text)
}

// subscriptValueOfReference is subscriptValue for a subscript that arrived as
// **text** rather than as a word the parser read: a builtin's operand, and a
// reference resolved at run time. `unset 'a[$i]'`, `read 'v[${#v}+1]'` and
// `v='x[$(echo 2)]'` all reach the interpreter with their brackets still in a
// string, and the `$` inside them is the script's own — written there and
// never expanded, because the quotes are what kept it.
//
// So it is expanded here, once. That is the same substitution the reader used
// to perform for everyone, and these are the callers it was right for:
// measured on zsh 5.9.2, `i=2; v='x[$i]'` and `v='x[$(echo 2)]'` both read the
// second element, so a substitution written into a resolved reference is
// performed when the reference is read (#1852).
//
// A subscript that came out of a *word* is the opposite case and is read as it
// stands — it is already a result, and expanding it again ran what the first
// round had only produced (#3047).
func (r *Runner) subscriptValueOfReference(text string) (int, error) {
	return r.subscriptValueAsWritten(text, r.expandArithText(text))
}

// subscriptValueAsWritten is subscriptValue told what the *source* spelled,
// which decides whether a separator still in the expanded text ends the
// expression or is the arithmetic operator — see subscriptExpression.
func (r *Runner) subscriptValueAsWritten(written, text string) (int, error) {
	if err := r.emptySubscriptText(text); err != nil {
		return 0, err
	}
	return r.expressionValue(r.subscriptExpression(written, text))
}

// subscriptExpression is the part of a subscript's text an expression reads,
// which is not always the whole of it: one dialect stops at the first
// top-level `,` or `;` and discards the rest. See
// Semantics.SubscriptExpressionStopsAtASeparator, where the measurements are.
//
// A separator standing *first* leaves the text whole, because truncating it
// would make the empty expression — which is zero and an answer — where the
// shell complains about the character.
//
// A **comma the source wrote** is the operator and does not end anything:
// measured, `a=(1 2 3); a[1,2,3]=(x y)` on zsh 5.9.2 is the span 1 through
// the arithmetic `2,3`, which is 3. So only a comma that arrived through a
// substitution ends the expression, which is the same rule that decides
// where a *pair* is separated. A `;` ends it either way — it is no part of
// any arithmetic — and that is what gives `${a[2,3;5]}` its second end of 3.
func (r *Runner) subscriptExpression(written, text string) string {
	at := syntax.SubscriptExpressionEnd(text)
	if at <= 0 {
		return text
	}
	if text[at] == ',' && syntax.SubscriptExpressionEnd(written) >= 0 {
		return text
	}
	if !r.ask(r.sem().SubscriptExpressionStopsAtASeparator,
		"a subscript's expression ending at a separator the source did not write") {
		return text
	}
	return text[:at]
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
	if n, err := strconv.Atoi(strings.TrimSpace(text)); err == nil {
		return n, nil
	}
	// Read as written, blanks and all. The complaint quotes the text back and
	// the caller has only the untrimmed one to quote, so trimming here made
	// the two disagree: ksh93 answers `${a[ 1/0 ]}` with ` 1/0 : divide by
	// zero` and the trimmed text could not say the spaces had been there
	// (#2010). It also kept the offsets the parser records from lining up
	// with the text a diagnostic slices.
	// Read, not expanded again. Both callers hand over text their own word
	// expansion already produced, so a `$` still standing in it is a
	// character of the *result* rather than an expansion waiting to happen —
	// see arithTreeRead, where the panel rows are (#3047).
	tree, err := r.arithTreeRead(text)
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
//
// It also records that a **subscript** is what failed, which is what decides
// how much a give-up over it gives up: see Runner.badSubscript and
// Runner.giveUpForABadSubscript. Here rather than at each of the two dozen
// callers because wording one of these *is* that fact, and because several of
// them are two frames away from the give-up that reads it — behind an
// arithError a `$(( a[b c] ))` carries out of the evaluator. A caller that
// ends the script instead never reads the flag, and it is cleared with
// expandErr per command.
//
// The three callers with no brackets in front of them take
// Runner.expressionFailure, which is this wording without the mark. Written
// that way round on purpose: a subscript site added later is right by
// default, where a mark set at the call sites would have to be remembered at
// each of them — which is how a second helper came to omit a case three times
// in this package already.
func (r *Runner) subscriptFailure(text string, err error) string {
	r.badSubscript = true
	return r.expressionFailure(text, err)
}

// expressionFailure is subscriptFailure's wording for an expression that is
// **not** a subscript: a substring range's end, a `printf` numeric operand,
// and the value of a name re-read as an expression. The sentence is the same
// one — every shell measured says about `${a[b c]}` exactly what it says
// about `$((b c))` — and the give-up is not, which is the whole of why there
// are two doors.
//
// Measured 2026-09-17 on bash 5.3.20 and 3.2.57, as a script file and as one
// `-c` string, `echo "next=$?"; echo end` behind each on its own lines:
// `${v:b c:2}` writes the complaint and then `next=1` and `end` by **both**
// routes, where `${a[b c]}` — the same arithmetic, in brackets — writes
// nothing after the complaint under `-c` and exits 1 (#3502).
func (r *Runner) expressionFailure(text string, err error) string {
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
	return r.subscriptIndexAsWritten(text, text)
}

// subscriptIndexAsWritten is subscriptIndex told what the source spelled, for
// the reason subscriptValueAsWritten exists.
func (r *Runner) subscriptIndexAsWritten(written, text string) (int, bool) {
	n, err := r.subscriptValueAsWritten(written, text)
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
	name = r.throughNameref(name)
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
	if r.wholeArrayIndex(e) || dotRanged(e) {
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
	//
	// The width letters are in the list because the column that has them
	// stores the presentation, which is a change attributeWouldChange reports
	// — measured, `typeset -a n; typeset -L 3 n; n=(abcd efgh)` is `(abc
	// efg)` on ksh93u+ (#2859).
	_, width := r.fieldWidth[name]
	if !r.integer[name] && !r.lowered[name] && !r.uppered[name] && !width {
		return a
	}
	changed := false
	for _, sub := range a.subscripts() {
		if a[sub].Nested != nil {
			// A nested array is not a value this name's attribute reaches on
			// a *write*. Measured 2026-09-14 on ksh93u+, the one column whose
			// elements nest: `typeset -i a; a[1]=(5+5)` is `typeset -a -i
			// a=([1]=(5+5) )` there, unevaluated, and `a[1]=(2+2 3+3)` keeps
			// both words as text — where the same attribute *arriving* over a
			// standing `a[1]=(5+5)` does fold it, to `([1]=(10) )`. So the
			// literal's words are not the name's values and the arrival is a
			// different question; see foldedElems, which is the arrival and
			// does recurse. An element assignment's value still folds, and
			// reaches the attribute where it is written rather than here —
			// `typeset -i a[1][2]=5+5` is `([2]=10)` (#2491).
			continue
		}
		if r.attributeWouldChange(name, a[sub].scalar()) {
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
		if a[sub].Nested != nil {
			// The fold goes *into* a nested array rather than flattening it.
			// Measured on the one column whose elements nest: `a[1]=(5+5);
			// typeset -i a` is `typeset -a -i a=([1]=(10) )` there, the
			// nesting kept and the word inside it evaluated. Taking the
			// element's scalar reading instead — which is its first element,
			// or the empty string where it has none — folded a whole array
			// into one number and lost every other element it held, silently
			// (#2491).
			folded[sub] = Element{Nested: r.foldedElems(name, a[sub].Nested)}
			continue
		}
		v, ok := r.attributeFolded(name, a[sub].scalar())
		if !ok {
			// The integer evaluation failed and has already said so, which
			// is the one case attributeFolded stores nothing for. The array
			// is left as it stands rather than half rewritten.
			return a
		}
		folded[sub] = Scalar(v)
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
			a[i] = Scalar(v)
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

// unsetFlaggedSubscript is `unset 'a[(r)y]'`, where the operand's subscript
// opens with a flag group and names its element by *searching*.
//
// handled is false where there is no group, which is every other operand and
// every grammar without them.
//
// The operand arrives as a **runtime string** rather than as a parsed word,
// which is why this was left out of the change that answered the same group
// on the left of an assignment (#1102): the group could be scanned off the
// text, but its operand had never been lexed and so had nowhere to hang. It
// is read as the reference it is instead — one parse, from which the group,
// its operand and the plain arithmetic reading all come — so `unset 'b[(r)y]'`
// searches and `unset 'b[(r)$w]'` searches for what `$w` holds. Measured on
// zsh 5.9.2, the one shell with the construct, with `b=(x y z)`:
//
//	unset 'b[(r)y]'         x  z   the element whose value matched
//	unset 'b[(re)y]'        x  z   and exact matching finds the same one
//	unset 'b[(r)y*]'        x  z   the operand is a pattern
//	unset 'b[(i)y]'         x  z   the index form names the same element
//	unset 'b[(R)x]' on (x y x)  x y   the reverse search takes the last
//	unset 'b[(r)nomatch]'   x y z  a miss is one past the last, so nothing
//	unset 'b[(e)2]'         x  z   no search flag, so an ordinary subscript
//	unset 's[(r)l]' on hello   helo   a search over a string is a character
//
// What is left behind is the dialect's answer and not this one's — three
// elements with an empty one in the middle, which is
// UnsetArraySpanLeavesOneEmptyElement — so all this decides is *which*
// element, exactly as the assignment's group does.
//
// An association is answered before this is reached, and is measured to want
// that: `unset 'm[(r)v]'` changes nothing there, the brackets being a key.
func (r *Runner) unsetFlaggedSubscript(base, operand, sub string) (handled bool, code int) {
	e, ok := r.reference(operand)
	if !ok || e.IndexFlags == nil {
		return false, 0
	}
	idx, named := r.flaggedTargetIndex(e, false)
	if !named {
		// Refused by name, and the refusal has already been written.
		return true, 1
	}
	return true, r.unsetArrayElem(base, idx, sub)
}

// unsetEmptiesAnUnwrittenArray is what removing one element does, in a
// subshell, to an array the subshell has only been *looking at*.
//
// One column empties it: the rest of that subshell sees nothing of the array
// at all, and the parent's copy is untouched. Measured 2026-09-17,
// `env -i PATH=/usr/bin:/bin LC_ALL=C HOME=$d`, stdin /dev/null, a fresh
// directory, ksh93u+ (AT&T 2012), a script file and again under `-c`:
//
//	a=(1 2 3)
//	( unset "a[1]"; echo "in=[${a[*]}] n=${#a[@]}" )   in=[] n=0
//	echo "after=[${a[*]}]"                             after=[1 2 3]
//
// Four rows say what it is about, and each of them removes a candidate:
//
//	( echo "[${a[*]}]" )                    [1 2 3]  — the copy is fine
//	( a[0]=9; echo "[${a[*]}]" )            [9 2 3]  — a write is fine
//	( a[0]=9; unset "a[1]"; echo … )        [9 3]    — a write first, and the
//	                                                   unset is ordinary
//	( b=(1 2 3); unset "b[1]"; echo … )     [1 3]    — an array the subshell
//	                                                   made is its own
//
// So it is an element unset of an array this subshell has not written to, and
// it is indexed arrays only: `typeset -A m; m[k]=v; m[j]=w; ( unset "m[k]";
// echo "[${m[@]}]" )` leaves the other key standing there, and a plain
// `unset a` and a scalar's `unset v` are ordinary everywhere. A nested
// subshell answers for itself — the inner one empties its own view and the
// outer one still has all three.
//
// **And it is those two contexts and not every subshell**, which is measured
// rather than assumed: the array is left alone in a background job — `( … ) &`
// and `{ …; } &` alike — at either end of a pipeline, and in a process
// substitution, all of which are contexts that shell forks. An explicit
// `( … )` and a `$( … )` are the two it does not, and they are the two that
// empty. So the flag is set at those two sites rather than read off
// r.inSubshell, which every clone sets.
//
// bash 5.3.20 and zsh 5.9.2 both remove the one element and leave the
// parent's array whole, which is what this did in every dialect (#3517).
//
// Measured on the only ksh93 on this machine, which is AT&T's 2012 build —
// the same binary the oracle panel's ksh column is, so the preset matches the
// column it is graded against.
func (r *Runner) unsetEmptiesAnUnwrittenArray(name string) bool {
	if !r.arraysAreAView || r.subshellWroteArrays[name] {
		return false
	}
	if !r.ask(r.sem().UnsetElementEmptiesAnUnwrittenArrayInASubshell,
		"an element unset emptying an array a subshell has only inherited") {
		return false
	}
	// Emptied rather than removed: the name is still an array, and
	// `${#a[@]}` reads 0 rather than the name being gone.
	r.storeArray(name, Array{})
	return true
}

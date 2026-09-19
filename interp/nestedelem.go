// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
	"strings"

	"github.com/blairham/sh/syntax"
)

// nestElemLiteral is `a[i]=(p q)` where the literal becomes the element's own
// value rather than words spliced into the array around it.
//
// ksh93's answer, and the one the store had no shape for until Element grew a
// Nested field. The array keeps its length and the element stops being a
// string: `a=(x y); a[1]=(p q)` is still two elements, `${a[1]}` is `p`, and
// `typeset -p a` prints `typeset -a a=(x (p q) )`.
//
// Measured on ksh93u+ 2012-08-01, 2026-09-13, `env -i` with a scratch HOME:
//
//	a=(x y);   a[1]=(p q)   ${a[@]} -> x p     n=2
//	a=(x y z); a[1]=()      ${a[@]} -> x, an empty one, z    n=3
//	a=(x y);   a[1]+=(p)    typeset -a a=(x (p) )     the element was a string
//	a=(x y);   a[1]=(p q); a[1]+=(r)   typeset -a a=(x (p q r) )   and now not
//	s=abc;     s[1]=(p q)   typeset -a s=(abc (p q) )
//	unset a;   a[3]=(x y)   ${a[@]} -> x       n=1, the gap is no element
//	typeset -A h; h[a,b]=(x y)   typeset -A h=([a,b]=(x y) )
//
// The nested words are reachable both ways, and this comment said otherwise
// for longer than it was true. A second subscript in an **expansion** reads
// them — measured 2026-09-19, `a=(x y); a[1]=(p q)` then `${a[1][1]}` is `q`
// and `${a[1][0]}` is `p` here and on ksh93u+ 2012-08-01 alike, with the
// listing byte-identical beside them — which is what #2830 built and what
// interp/nestedchainsub.go is. It was a parse error when this construct
// landed, so `typeset -p` was the only way to see what had been stored; a
// comment left saying so makes the shell look less finished than it is. A
// second subscript on the left of an **assignment** is read too, and builds
// this same value by the other route: see interp/chainassign.go, which is
// `a[1][2]=v` (#2491).
//
// No range reading is asked for and that is not an omission: this dialect
// reads a subscript's comma as the arithmetic operator whose value is its
// right operand, so `a[2,3]=(x y)` is `a[3]=(x y)` — the same reading
// `a[2,3]=x` already takes here, reached by the same single-subscript route.
func (r *Runner) nestElemLiteral(a *syntax.Assign) {
	value, ok := r.nestedLiteral(a.Name, a.Elems)
	if !ok {
		return
	}
	if r.assocDeclared(a.Name) {
		// A keyed table takes the same value under a key, which is the row
		// `h[a,b]=(x y)` measures — and it is not refused as a slice the way
		// the splicing dialect refuses it, because nothing here replaces a
		// span.
		key, keyed := r.assocAssignKey(a.Name, a.Index)
		if !keyed {
			return
		}
		r.storeAssocElement(a.Name, key, value)
		return
	}
	text := r.joinWord(a.Index)
	// The subscript as written is what a boundary refusal quotes back, and it
	// is not the text the arithmetic reads — see subscriptSubject.
	subject := subscriptSubject(a.IndexText, text)
	idx, err := r.subscriptValueAsWritten(subject, text)
	if err != nil {
		r.fatal("%s\n", r.subscriptFailure(text, err))
		return
	}
	r.storeArrayElement(a.Name, idx, subject, value, a.Append)
}

// nestedLiteral is the value a literal standing in an element's place builds.
//
// Through literalInto, the placement the whole-array spelling uses, so that a
// literal placing its own elements — `a[1]=([2]=z)` — nests an array with a
// gap in it rather than a dense one. Two spellings of one construct must not
// come to disagree about what a literal *is*; see literalWords, which is the
// same call for the dialect that splices.
func (r *Runner) nestedLiteral(name string, elems []*syntax.Word) (Element, bool) {
	parsed, ok := r.literalElems(elems, r.literalShapeReadsSubscripts(elems))
	if !ok {
		return Element{}, false
	}
	built, ok := r.literalInto(name, Array{}, 0, parsed)
	if !ok {
		return Element{}, false
	}
	// An empty literal is an empty *array* and not a nil one: `a=(x y z);
	// a[1]=()` leaves three elements there, the middle one reading back as a
	// newline between two parens. A nil Nested would have made it a string.
	if built == nil {
		built = Array{}
	}
	return Element{Nested: built}, true
}

// storeArrayElement puts a whole value into one element of an indexed array,
// promoting a name that holds a string and appending where the operator says
// so.
//
// Not setArrayElem with an Element argument: that path asks
// subscriptSplicesCharacters, which is the dialect where a subscript on a
// string names a character — a question this construct never reaches, since
// the one dialect that nests promotes the string instead. `s=abc; s[1]=(p q)`
// is `typeset -a s=(abc (p q) )` there and not a splice into `abc`.
func (r *Runner) storeArrayElement(name string, idx int, subject string, value Element, appendTo bool) {
	a := r.elementsOfName(name, idx)
	if r.unspecified {
		return
	}
	pos, within := r.elemPos(a, idx)
	if !within {
		if idx < 0 && r.ask(r.sem().NegativeSubscriptPastTheStartInserts,
			"a negative subscript past the first element placing one in front of it") {
			r.storeArray(name, insertAtTheFront(a, value))
			return
		}
		if r.unspecified {
			return
		}
		r.fatal("%s\n", Wording(r.diag().BadArraySubscript,
			"%[1]s[%[2]s]: bad array subscript", name, subject))
		return
	}
	a[pos] = nestedAppended(a[pos], value, appendTo)
	r.storeArray(name, a)
}

// storeAssocElement is the same write into a keyed table.
//
// A **produced** table is written through its own hook and never into the
// stored one, which is setAssocElem's rule and not a nicety: a stored table
// shadows the producer, so one assignment would turn a live view into a
// snapshot taken at that instant. The hook takes a string, so what it is
// handed is the element's scalar reading — the same bytes every other reader
// of a nested element gets. Reaching the producer with a flattened value beats
// storing the whole one where nothing will ever read it again.
func (r *Runner) storeAssocElement(name, key string, value Element) {
	if write, produced := r.dynamicAssocWriters[name]; produced {
		write(r, key, value.scalar(), true)
		return
	}
	a := r.AssocArrays[name]
	if a == nil {
		a = AssocArray{}
		if r.AssocArrays == nil {
			r.AssocArrays = map[string]AssocArray{}
		}
		r.AssocArrays[name] = a
	}
	a[key] = value
	// Written to, so the name leaves the declared-only set — the same note
	// setAssocElem makes, and for the same reason.
	r.compoundWasAssigned(name)
	r.nameIsBack(name)
}

// nestedAppended is what `a[i]+=(p)` leaves, against what `a[i]=(p)` leaves.
//
// Measured, and the two halves do not follow from each other:
//
//	a=(x y); a[1]+=(p)               typeset -a a=(x (p) )
//	a=(x y); a[1]=(p q); a[1]+=(r)   typeset -a a=(x (p q r) )
//
// So the operator appends only where there is a nested array to append to. An
// element holding a *string* is replaced outright rather than becoming the
// first element of a new nested array — the `y` in the first row is gone, not
// nested — which is the row a symmetry argument gets wrong.
//
// The nested array is copied rather than written through, because an element
// read out of the store may be the one a saved scope is holding: see
// clonetables.go, where the outer maps are cloned a level at a time.
func nestedAppended(held, value Element, appendTo bool) Element {
	if !appendTo || held.Nested == nil {
		return value
	}
	out := make(Array, len(held.Nested)+len(value.Nested))
	for k, v := range held.Nested {
		out[k] = v
	}
	next := out.pastTheEnd()
	for _, k := range value.Nested.subscripts() {
		out[next] = value.Nested[k]
		next++
	}
	return Element{Nested: out}
}

// listedElement is one element as a listing writes it: the quoted string, or
// the nested array spelled out.
//
// Nesting can only reach a listing in the one dialect whose axis builds it, so
// there is no question to ask here — an element that holds an array is written
// as an array wherever it turns up.
func (r *Runner) listedElement(e Element) string {
	if e.Nested == nil {
		return r.declareQuoted(e.Str)
	}
	return r.nestedListing(e.Nested)
}

// nestedListing is a nested array as `typeset -p` writes it.
//
// The gap rule is the outer array's, reached by the same test: measured,
// `a=(x y); a[1]=([2]=z)` lists as `typeset -a a=(x ([2]=z) )`, so a nested
// array with a hole in it writes its subscripts exactly as a top-level one
// does. Written recursively rather than one level deep, because nothing in the
// construct stops at one.
func (r *Runner) nestedListing(a Array) string {
	subs := a.subscripts()
	gaps := r.arrayHasGaps(a)
	elems := make([]string, 0, len(subs))
	for _, i := range subs {
		if gaps {
			elems = append(elems, fmt.Sprintf("[%d]=%s", i, r.listedElement(a[i])))
		} else {
			elems = append(elems, r.listedElement(a[i]))
		}
	}
	return "(" + strings.Join(elems, " ") + nestTrailingSpace(a.lastElement()) + ")"
}

// nestTrailingSpace is the one oddity in ksh93's listing of a nested array: a
// list whose **last** element holds a non-empty array is written with a space
// before the closing paren, and one whose last element is anything else is
// not.
//
// Measured rather than reasoned, on ksh93u+ 2012-08-01 through `od -c`, and it
// took six rows to state — every one of them is a row a simpler rule gets
// wrong:
//
//	a=(x y);   a[1]=(p q)              typeset -a a=(x (p q) )
//	a=(x y);   a[0]=(p q)              typeset -a a=((p q) y)
//	a=(1 2 3); a[1]=(p q)              typeset -a a=(1 (p q) 3)
//	a=(x y);   a[1]=()                 typeset -a a=(x ())
//	a=(x y);   a[1]=(p q); a[0]=(m n)  typeset -a a=((m n) (p q) )
//	a=(x y);   a[5]=(p q)              typeset -a a=([0]=x [1]=y [5]=(p q) )
//
// A rule that gave every nested element a trailing space writes two spaces in
// the fifth row; a rule that gave the list one unconditionally writes it in
// the second, third and fourth.
//
// Takes the last element rather than the list, so that the indexed and the
// keyed listing share the rule instead of each carrying a copy of it — a table
// nests too: `typeset -A h; h[a,b]=(x y)` is `typeset -A h=([a,b]=(x y) )`.
//
// No axis: only the dialect that nests can produce an element this answers
// for, so no other column can reach it.
func nestTrailingSpace(last Element, any bool) string {
	if any && len(last.Nested) > 0 {
		return " "
	}
	return ""
}

// lastElement is the element a listing writes last, and whether there is one.
func (a Array) lastElement() (Element, bool) {
	subs := a.subscripts()
	if len(subs) == 0 {
		return Element{}, false
	}
	return a[subs[len(subs)-1]], true
}

// lastElement is the same for a keyed table, under the last key this
// implementation lists — which is its own sorted order and not the shell's,
// since none of them promises one. See AssocArray.keys.
func (a AssocArray) lastElement() (Element, bool) {
	keys := a.keys()
	if len(keys) == 0 {
		return Element{}, false
	}
	return a[keys[len(keys)-1]], true
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strconv"
	"strings"

	"github.com/blairham/sh/syntax"
)

// literalElem is one element of `a=(…)` as it was written.
//
// Two shapes share the parentheses: `[sub]=value` names where the value goes,
// and a bare word takes the next position going. The shape is decided on the
// word before any expansion, for the reason `assocElem` gives — expanding
// first would hand `[2]=c` to the pattern matcher, where it is a character
// class.
type literalElem struct {
	// sub and value are the two halves of a `[sub]=value` element, each
	// already expanded as an assignment's value: no splitting and no globbing,
	// so `a=([2]=$x)` is one element however many words `$x` holds.
	sub, value string
	// subscripted distinguishes the shapes. A bare element's fields are in
	// `fields` and both strings are empty.
	subscripted bool
	// fields is what a bare element expanded to, which may be any number of
	// words: an array is built from a command's output that way.
	fields []string
}

// literalElems expands an array literal's elements once.
//
// Once matters: the value of an element may have side effects — `[$((i++))]=v`
// — so deciding the shape and then re-expanding to place it would run them
// twice.
func (r *Runner) literalElems(elems []*syntax.Word) []literalElem {
	out := make([]literalElem, 0, len(elems))
	for _, w := range elems {
		if sub, value, ok := r.assocElem(w); ok {
			out = append(out, literalElem{sub: sub, value: value, subscripted: true})
			continue
		}
		out = append(out, literalElem{fields: r.expandWord(w)})
	}
	return out
}

// assignArrayLiteral is `a=(…)` and `a+=(…)` on a name with no associative
// attribute.
//
// A subscripted element places its value where it says rather than becoming
// one, which is the ordinary way to build a sparse array: `a=([2]=c [0]=a)`
// is two elements, at 0 and 2, with nothing between them. It used to keep the
// text — `${a[0]}` answered the six characters `[2]=c` — and nothing reported
// it, so the array looked populated and was not.
func (r *Runner) assignArrayLiteral(name string, elems []*syntax.Word, appendTo bool) {
	parsed := r.literalElems(elems)
	if r.literalSubscriptIsAKey(parsed) {
		// The subscript is text rather than an expression, and a literal
		// written with one declares a keyed array — one concept with two
		// consequences, so the elements go where a declared name's would.
		r.markAssoc(name)
		r.assignAssocElems(name, parsed, appendTo)
		return
	}

	a := Array{}
	next := 0
	if appendTo {
		if old, ok := r.Arrays[name]; ok {
			a = old
			// After the highest subscript rather than after the count:
			// appending to `a[0]=x a[5]=y` puts the next element at 6, which
			// is where the end is.
			next = old.pastTheEnd()
		}
	}
	for _, e := range parsed {
		if !e.subscripted {
			for _, f := range e.fields {
				a[next] = f
				next++
			}
			continue
		}
		idx, err := r.subscriptValue(e.sub)
		if err != nil {
			// The last of the places a subscript is read, and the one #649
			// missed: it kept a wording of its own and carried on, where the
			// two shells that evaluate a literal's subscript report the
			// arithmetic failure and end the script. The third reads the
			// text as a key and never reaches this.
			r.fatal("%s\n", r.subscriptFailure(e.sub, err))
			return
		}
		pos, ok := r.elemPos(a, idx)
		if !ok {
			// Before the first element. Refused and fatal, as the plain form
			// is — and worded apart from it by both shells that get here:
			// one names the element as written and the other the subscript.
			wording := r.diag().BadArrayLiteralSubscript
			if wording == "" {
				wording = r.diag().BadArraySubscript
			}
			r.fatal("%s\n", Wording(wording,
				"%[1]s[%[2]s]: bad array subscript", name, e.sub, e.value))
			return
		}
		a[pos] = e.value
		// A bare element after a subscripted one continues from there rather
		// than from where the count had reached: `a=(x [3]=y z)` puts z at 4.
		// Measured in both shells that accept the mixture, and it follows the
		// *written* subscript through the base, so the same literal fills the
		// same positions whichever number the first element answers to.
		next = pos + 1
	}
	r.storeArray(name, a)
}

// literalSubscriptIsAKey asks whether a subscript inside a literal is the text
// between the brackets or an expression to evaluate.
//
// Asked only where the two readings differ. A subscript spelled as a plain
// decimal numeral evaluates to itself, so `a=([2]=c)` fills the same slot
// either way and the common form asks nothing — which is what keeps the
// construct usable in a core that has chosen no shell. `[1+1]`, `[i]` and
// `[k]` are where the answers part.
func (r *Runner) literalSubscriptIsAKey(parsed []literalElem) bool {
	for _, e := range parsed {
		if !e.subscripted || isDecimalSubscript(e.sub) {
			continue
		}
		return r.ask(r.sem().ArrayLiteralSubscriptIsAKey,
			"a subscript inside an array literal being a key rather than an expression")
	}
	return false
}

// isDecimalSubscript reports whether the text is a plain decimal integer, and
// so means the same thing read either way.
func isDecimalSubscript(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	_, err := strconv.Atoi(s)
	return err == nil
}

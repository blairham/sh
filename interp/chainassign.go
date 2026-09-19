// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// An assignment whose name carries more than one subscript — `a[1][2]=v` —
// where each subscript after the first reaches *into* the value the one
// before it named.
//
// One dialect has it, and what it builds is a value this store already had:
// `a[1][2]=v` and `a[1]=([2]=v)` leave the same thing, which is
// `typeset -a a=([1]=([2]=v) )` with `${#a[@]}` of 1. So the work here is the
// walk down and not a new kind of element — see interp/nestedelem.go, where
// Element.Nested and the listing that writes it back came from.
//
// Measured 2026-09-14 on ksh93u+ 2012-08-01, `env -i` with a scratch HOME,
// read back with `typeset -p`:
//
//	a[1][2]=v                     typeset -a a=([1]=([2]=v) )
//	a[1][2]=v; a[1][3]=w          typeset -a a=([1]=([2]=v [3]=w) )
//	a[1][2]=v; a[2][0]=w          typeset -a a=([1]=([2]=v) [2]=(w) )
//	a[1][2][3]=v                  typeset -a a=([1]=([2]=([3]=v) ) )
//	a=(x y);   a[1][2]=v          typeset -a a=(x ([0]=y [2]=v) )
//	a[1]='';   a[1][2]=v          typeset -a a=([1]=([0]='' [2]=v) )
//	a=(p q r); a[1][0]=v          typeset -a a=(p (v) r)
//	s=abc;     s[1][2]=v          typeset -a s=(abc ([2]=v) )
//	typeset -A m; m[k][2]=v       typeset -A m=([k]=([2]=v) )
//	typeset -A m; m[k][x]=v       typeset -A m=([k]=(v) )
//	a[1][2]+=v                    typeset -a a=([1]=([2]=v) )
//	a[1][2]=v; a[1][2]+=Q         typeset -a a=([1]=([2]=vQ) )
//
// Four things fall out of those rows and none of them is guessable from the
// others:
//
//   - **A string already in an element is promoted rather than replaced.**
//     `a=(x y); a[1][2]=v` keeps `y` at the base of the array it builds under
//     element 1, which is appendedOverAScalar's rule one level down. An
//     element holding the *empty* string is promoted too, and an element that
//     is not there is not — the same distinction, drawn by whether the store
//     has an entry rather than by what is in it.
//   - **Only the first subscript can be a key.** `m[k][x]=v` stores under `k`
//     and then evaluates `x` as arithmetic, which with `x` unset is 0. The
//     table attribute is the *name's*, and a nested array is an array
//     whatever holds it.
//   - **The last subscript is an ordinary element write**, append included:
//     `a[1][2]+=Q` joins what is under 2 of the nested array, exactly as
//     `a[2]+=Q` joins what is under 2 of an ordinary one.
//   - **The chain is any depth.** `a[1][2][3]=v` nests twice, so this is a
//     walk rather than a special case for two.
//
// What `${a[1][2]}` *reads* is a second question and is not this: the grammar
// for a chained expansion exists and means something else in the shell that
// has it — see syntax.Dialect.ChainedSubscript, and #2830 for the reading this
// one would need.

// assignChainedElement is `a[i][j]…=v`, where Assign.Leading holds every
// subscript but the last.
func (r *Runner) assignChainedElement(a *syntax.Assign) {
	held, there, place, ok := r.chainRoot(a)
	if !ok {
		return
	}
	rest := make([]string, 0, len(a.Leading))
	for _, link := range a.Leading[1:] {
		rest = append(rest, r.joinWord(link.Index))
	}
	r.chainWrite(a.Name, held, there, place, rest, r.joinWord(a.Index),
		r.assignValue(a), a.Append)
}

// chainWrite walks the subscripts after the first and writes the value under
// the last of them.
//
// held is what the first subscript named and place puts it back; the two of
// them are how the one link that is not an element of an array — the name's
// own — is spent before the loop, so that everything here is one rule however
// deep the chain goes and whichever route reached it. The plain `a[1][2]=v`
// and the declaration `typeset a[1][2]=v` are the same value in the shell
// that has them, so they are the same walk here.
//
// Every subscript below the first is arithmetic, table attribute or no:
// `typeset -A m; m[k][x]=v` stores under the key `k` and then evaluates `x`,
// which with `x` unset is 0. The attribute is the *name's*, and what a link
// indexes is an array.
func (r *Runner) chainWrite(name string, held Element, there bool, place func(Element),
	rest []string, last, value string, appendTo bool,
) {
	for _, text := range rest {
		inner := nestedForWrite(held, there, r.arrayBase())
		idx, err := r.subscriptValue(text)
		if err != nil {
			r.fatal("%s\n", r.subscriptFailure(text, err))
			return
		}
		outer, outerPlace := inner, place
		held, there = inner[idx]
		place = func(e Element) {
			outer[idx] = e
			outerPlace(Element{Nested: outer})
		}
	}
	inner := nestedForWrite(held, there, r.arrayBase())
	idx, err := r.subscriptValue(last)
	if err != nil {
		r.fatal("%s\n", r.subscriptFailure(last, err))
		return
	}
	// The last subscript is an ordinary element write, append included:
	// `a[1][2]+=Q` joins what is under 2 of the nested array exactly as
	// `a[2]+=Q` joins what is under 2 of an ordinary one.
	if appendTo {
		value = inner[idx].scalar() + value
	}
	// The value goes through the *name's* attributes, which is where an
	// element assignment's value meets them wherever it is written: measured,
	// `typeset -i a[1][2]=5+5` is `([2]=10)` there. storeArray's fold cannot
	// do it, because what it is handed by the time this lands is a nested
	// array — and a nested array is not a value that attribute reaches, which
	// is the row `typeset -i a; a[1]=(5+5)` keeping its text says.
	if folded, ok := r.attributeFolded(name, value); ok {
		value = folded
	} else {
		return
	}
	inner[idx] = Scalar(value)
	place(Element{Nested: inner})
}

// chainRoot resolves the first subscript against the *name*, which is the one
// link that is not an element of an array.
func (r *Runner) chainRoot(a *syntax.Assign) (Element, bool, func(Element), bool) {
	first := a.Leading[0]
	if r.refuseReadonly(a.Name, assignedAlone) {
		return Element{}, false, nil, false
	}
	if r.assocDeclared(a.Name) {
		// A declared table takes its subscript as a key, expanded and never
		// evaluated — the same switch the attribute throws for a plain
		// `m[k]=v`, and the reason the links below it cannot be keys: what
		// they index is an array.
		key, ok := r.assocAssignKey(a.Name, first.Index)
		if !ok {
			return Element{}, false, nil, false
		}
		return r.chainRootInTable(a.Name, key)
	}
	text := r.joinWord(first.Index)
	idx, err := r.subscriptValue(text)
	if err != nil {
		r.fatal("%s\n", r.subscriptFailure(text, err))
		return Element{}, false, nil, false
	}
	return r.chainRootInArray(a.Name, idx, subscriptSubject(first.Text, text))
}

// chainRootInTable and chainRootInArray are the two roots a chain can have,
// reached from the assignment route above and from the declaration route in
// declareElement. Written here rather than at either caller because a chain
// that started from a declaration must land in the same place as the same
// chain written as a plain assignment.
//
// Neither asks the readonly refusal: a name is refused once, by the route that
// reached it, and the declaration route has already asked with a wording and a
// fatality of its own.
func (r *Runner) chainRootInTable(name, key string) (Element, bool, func(Element), bool) {
	held, there := Element{}, false
	if tbl, ok := r.assocFor(name); ok {
		held, there = tbl[key]
	}
	return held, there, func(e Element) { r.storeAssocElement(name, key, e) }, true
}

func (r *Runner) chainRootInArray(name string, idx int, subject string) (Element, bool, func(Element), bool) {
	// elementsOfName is what promotes a scalar the name is holding — `s=abc;
	// s[1][2]=v` is `typeset -a s=(abc ([2]=v) )`, with `abc` kept at the
	// base — and it is the same call the single-subscript store makes, so the
	// promotion cannot come to differ between the two spellings.
	arr := r.elementsOfName(name, idx)
	if r.unspecified {
		return Element{}, false, nil, false
	}
	pos, within := r.elemPos(arr, idx)
	if !within {
		r.fatal("%s\n", Wording(r.diag().BadArraySubscript,
			"%[1]s[%[2]s]: bad array subscript", name, subject))
		return Element{}, false, nil, false
	}
	held, there := arr[pos]
	return held, there, func(e Element) {
		arr[pos] = e
		r.storeArray(name, arr)
	}, true
}

// nestedForWrite is the array a link writes in, given what the element above
// it was holding.
//
// The three states are three answers and the middle one is the measured
// surprise: an element holding a *string* keeps that string at the base of
// the array built over it, where a whole-element literal — `a=(x y);
// a[1]=(p q)` — replaces it outright. So this is not nestedAppended's rule
// under another name, and the two are deliberately not folded.
func nestedForWrite(held Element, there bool, base int) Array {
	if held.Nested != nil {
		return held.Nested
	}
	if !there {
		// The element is not there at all, which is a different state from
		// one holding the empty string: `a[1]=""; a[1][2]=v` keeps the empty
		// string at the base and `a[1][2]=v` on its own does not.
		return Array{}
	}
	return Array{base: Scalar(held.Str)}
}

// declareChainedElement is the same walk reached from a declaration's
// operand — `typeset a[1][2]=v` — where the subscripts arrive as text rather
// than as words and the first one's reading is the declaration's own.
//
// ksh93 answers the two spellings identically, so they share the walk rather
// than each having one: the first subscript is resolved the way a declaration
// resolves a single one, and everything below it is the same.
func (r *Runner) declareChainedElement(base string, leading []string, last, value string, appends, tableBefore bool) {
	key, evaluated, isKey := r.subscriptedOperandKey(base, leading[0], tableBefore)
	if r.unspecified {
		return
	}
	var (
		held  Element
		there bool
		place func(Element)
		ok    bool
	)
	switch {
	case isKey && !evaluated:
		held, there, place, ok = r.chainRootInTable(base, key)
	case isKey:
		// A table whose letter arrived too late to be read: the subscript is
		// evaluated and the number it came to is the key, exactly as it is
		// for a single-subscript operand.
		idx, err := r.subscriptValue(leading[0])
		if err != nil {
			r.fatal("%s\n", r.subscriptFailure(leading[0], err))
			return
		}
		held, there, place, ok = r.chainRootInTable(base, itoa(idx))
	default:
		idx, err := r.subscriptValue(leading[0])
		if err != nil {
			r.fatal("%s\n", r.subscriptFailure(leading[0], err))
			return
		}
		held, there, place, ok = r.chainRootInArray(base, idx, leading[0])
	}
	if !ok {
		return
	}
	r.chainWrite(base, held, there, place, leading[1:], last, value, appends)
}

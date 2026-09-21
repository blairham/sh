// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// What an assignment prefix's entry *is*, as against what it holds.
//
// One reading makes the prefix a fresh cell — a plain scalar the command sees
// in place of whatever the name was bound to, with none of its elements and
// none of its letters. The other writes over the binding that is already
// there, so the command is shown the old kind with one element or one letter
// changed. See Semantics.AssignmentPrefixMakesAFreshCell for the rows.
//
// The difference is not a spelling. Under the overlay, `foo=(asdf fdsa)`
// followed by `foo=bar ff` shows the body `${foo[*]}` as `bar fdsa` and
// `${#foo[@]}` as 2, and `declare -i foo=7` followed by `foo=bar ff` hands
// the body `0` — the prefix's word read as arithmetic because the name it
// displaced carried the letter (#4087).
//
// The take-back is the same under both answers and is not this file's: what
// the name held is saved before the prefix is applied and put back when the
// command ends. What this file adds is the two moments between them where the
// displaced binding has to be got back *early* — a declaration that keeps the
// entry, and one that takes it into a scope of its own. See
// Runner.prefixEntryTakesTheDisplacedShapeBack, which is both of them.

// prefixEntryIsFresh reports whether this prefix's entry starts from nothing,
// for a name whose binding says the two readings would part over it.
//
// **Asked at the disagreement and nowhere else.** A name holding a plain
// scalar with no letters on it reaches the same place under either answer —
// the prefix's value is what the command sees either way — so the question is
// not put, which is every prefix a script in the common language writes. dash
// and ash have no way to make a binding that could reach it and so answer
// nothing rather than refusing on a construct they do have.
//
// A **name reference** takes the question away rather than answering it. bash
// 5.3.20 answers `foo=bar ff` over a `declare -n foo=target` with `declare -n
// foo="target"`, keeping the reference, where bash 3.2.57 writes the plain
// scalar every other row gets — so the one column that agrees with itself
// everywhere else does not agree here, and it is a second measurement rather
// than a row of this one.
func (r *Runner) prefixEntryIsFresh(name string) bool {
	if !r.prefixDisplacesAKindOrALetter(name) {
		return false
	}
	return r.ask(r.sem().AssignmentPrefixMakesAFreshCell,
		"an assignment prefix making a fresh cell rather than writing over the binding it displaces")
}

// prefixDisplacesAKindOrALetter reports whether the name is bound to
// something an overlay would leave showing through: an array, a table, or any
// declaration attribute.
//
// The attributes are compared as the whole struct rather than letter by
// letter, and through the same reader a scope's shadow uses, because the two
// answers to "what is an attribute" have to be one answer — a letter added to
// the runner without a line here would be one the fresh cell silently kept.
// See Runner.captureAttributes and Runner.dropNameAttributes, which is the
// other end of the same list.
func (r *Runner) prefixDisplacesAKindOrALetter(name string) bool {
	if _, isRef := r.nameref[name]; isRef {
		// Not this question — see prefixEntryIsFresh.
		return false
	}
	if _, inArray := r.Arrays[name]; inArray {
		return true
	}
	if _, inTable := r.AssocArrays[name]; inTable {
		return true
	}
	return r.captureAttributes(name) != nameAttributes{}
}

// emptyPrefixEntry takes the kind and the letters off a name, so that the
// store about to happen lands in a plain scalar.
//
// The value itself is left alone because the store is what replaces it, and
// because an **append** written as a prefix joins what the displaced binding
// was showing: `foo=(asdf fdsa); foo+=bar ff` answers `declare -x foo=
// "asdfbar"` in bash 5.3.20 — a plain exported scalar holding element zero
// with the part joined on. So the join reads the old binding and the cell it
// stores into is the fresh one, which is why this stands between the two.
func (r *Runner) emptyPrefixEntry(name string) {
	delete(r.Arrays, name)
	delete(r.AssocArrays, name)
	r.dropNameAttributes(name)
}

// prefixEntryTakesTheDisplacedShapeBack undoes the fresh cell for a name
// whose binding the shell has to have back *before* the command ends, putting
// the elements and the letters on again and writing the prefix's value into
// them through the ordinary rules.
//
// Two callers, and one reason: the fresh cell is what the running command is
// shown, so anything that takes the entry out of the command's hands takes
// the displaced binding with it.
//
// A declaration that **keeps** the entry is the first. The keeping is not
// "leave the fresh scalar standing" — measured 2026-09-21 on bash 5.3.20 from
// a script file under `env -i` with a scratch HOME:
//
//	foo=(a b);         foo=bar readonly foo   declare -arx foo=([0]="bar" [1]="b")
//	declare -i foo=7;  foo=bar readonly foo   declare -irx foo="0"
//
// The array is back with element zero written, and the integer letter is back
// with `bar` read as arithmetic — which is the ordinary assignment the name
// would have taken on its own line. So what a declaration keeps is the
// prefix's *value*, delivered through the binding it displaced.
//
// A declaration that **shadows** the entry is the second, and the take-back
// rather than the value is what is at stake there: the scope saves what it
// finds and gives that back on return, so a scope that found the fresh cell
// would hand the shell a plain scalar where its array had been. See
// Runner.prefixEntryShadowed, which calls this before it saves anything.
//
// Called ahead of the attribute a `readonly` is about to record, because one
// that had already landed would refuse the write this makes.
func (r *Runner) prefixEntryTakesTheDisplacedShapeBack(name string) {
	u := r.freshenedPrefixEntry(name)
	if u == nil {
		return
	}
	// Once. Two attributes over one name — `declare -rx v` — reach the
	// keeping twice, and a binding put back twice would be put back over the
	// value the first one delivered.
	u.freshened = false
	value, _ := r.getVar(name)
	if u.inArray {
		if r.Arrays == nil {
			r.Arrays = map[string]Array{}
		}
		r.Arrays[name] = u.array.clone()
	}
	if u.inTable {
		if r.AssocArrays == nil {
			r.AssocArrays = map[string]AssocArray{}
		}
		r.AssocArrays[name] = u.table.clone()
	}
	r.restoreAttributes(name, u.attrs)
	r.setVar(name, value)
}

// freshenedPrefixEntry finds the live prefix holding a name in a cell of its
// own, innermost first, and nil where no prefix made one.
//
// The running builtin's prefix is looked at before the calls around it for
// the reason the take-back frames are searched in that order anywhere else:
// two prefixes can be live over one name at once, and the one the command in
// hand made is the one this is about.
func (r *Runner) freshenedPrefixEntry(name string) *savedVar {
	for i := range r.prefixHeldUndo {
		if u := &r.prefixHeldUndo[i]; u.name == name && u.freshened {
			return u
		}
	}
	for f := len(r.callPrefixes) - 1; f >= 0; f-- {
		undo := r.callPrefixes[f].undo
		for i := range undo {
			if u := &undo[i]; u.name == name && u.freshened {
				return u
			}
		}
	}
	return nil
}

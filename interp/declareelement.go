// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A declaration whose operand names an array element rather than a variable.
//
// `typeset a[1]=v` was accepted and did nothing: the operand was split at its
// `=`, `a[1]` was handed to the variable store as if the brackets were part of
// the name, and every read of `${a[1]}` afterwards was empty at status 0.
// Measured 2026-09-07 — bash 5.3, bash-as-sh, bash 3.2, ksh93u+ and zsh 5.9.2
// all create the element, and a plain `a[1]=v` on a line of its own was
// already right here, so the divergence was the declaration builtin's own
// path and not the array store's (#1203).
//
// What the declaration does to the *variable* beside writing the element is a
// different question, and the panel gives it more than one answer — see
// Semantics.SubscriptedOperandTakesALocalDeclaration and its two neighbors.
// An element is not a name, so a shell that would have to freeze, type or
// localize the whole array refuses the operand instead.

// declareElement writes the element a declaration's subscripted operand names.
//
// The attributes and the scope are applied to the *base* name, which is the
// variable they are about: `typeset -x a[1]=v` exports `a` and writes the
// element, and `typeset -A m; typeset m[k]=v` places a key in the table rather
// than an element numbered by whatever `k` evaluates to.
// Nothing comes back: a refusal has already set r.unspecified or ended the
// script, and a readonly name that will not take the write has already left
// r.assignFailed for the builtin's own status to read. A status of its own
// would have to be merged with those three at every call site and could only
// disagree with them.
func (r *Runner) declareElement(base, sub, value string, f declareFlags, shadows bool) {
	if r.elementDeclarationRefused(base, sub, f, shadows) {
		return
	}
	fresh := false
	if shadows && !f.global {
		// The array the element belongs to is what becomes local, and it has
		// to become local before the element is written or the write lands
		// on the caller's array and the shadow puts an empty one over it.
		fresh = r.shadowTypeset(base)
		if r.unspecified {
			return
		}
	}
	// After the shadow, for the reason biDeclare gives: these are the local
	// array's attributes and not the caller's (#1673).
	//
	// Where the dialect records them at all: one column gives a subscripted
	// operand's letters to nothing, so `typeset -x a[1]=v` there lists the
	// name without the `x` while a whole-name `typeset -x a=(p q)` lists it
	// with one. See Semantics.SubscriptedOperandCarriesTheAttributes.
	if f != (declareFlags{}) {
		// Asked only where there is a letter to record. `typeset a[1]=v`
		// names no attribute at all, so the two answers cannot part over it
		// and a dialect need not have one.
		if !r.ask(r.sem().SubscriptedOperandCarriesTheAttributes,
			"a subscripted operand's declaration recording its letters on the name") {
			f = declareFlags{}
		}
		if r.unspecified {
			return
		}
	}
	r.applyAttributes(base, f)
	// And the cell that shadow made holds none of the caller's elements, so
	// the subscript this declaration writes is the only one in it: measured,
	// `arr=(a b c); f(){ local arr[1]=z; }` lists `([1]="z")` and the same
	// line at the top level lists all three with `b` replaced. See
	// freshcell.go.
	// hasValue is true so the kind-change axis is not asked here: a
	// subscripted operand always carries one, and what a declaration with a
	// value does to a name already the *other* kind of compound is a
	// question of its own that the panel splits differently — see
	// compoundKindChanged.
	r.markDeclaredCompound(base, fresh, f, true)
	if r.refuseReadonly(base, assignedByDeclaration) {
		// A name already frozen refuses the element as it refuses the
		// variable, and by the base's name: `readonly a; typeset a[1]=v`
		// names `a`, which is the same rule `unset a[0]` follows.
		return
	}
	// The store's own complaints do not name the builtin, where the refusals
	// above do: measured, `export a[0]=v` and `typeset a[0]=v` in the shell
	// whose arrays start at one both say `a: assignment to invalid subscript
	// range` with no builtin in the location, and `readonly a[1]=v` from the
	// same shell says `readonly:` in it. Put aside and given back, the way
	// badSubscriptOperand and the readonly refusal already do it.
	outer := r.inBuiltin
	r.inBuiltin = ""
	defer func() { r.inBuiltin = outer }()
	if r.assocDeclared(base) {
		r.setAssocElem(base, sub, value)
	} else {
		idx, err := r.subscriptValue(sub)
		if err != nil {
			r.fatal("%s\n", r.subscriptFailure(sub, err))
			return
		}
		r.setArrayElem(base, idx, sub, value)
		if r.unspecified || r.ctl == controlExit {
			return
		}
	}
	if f.readonly && !f.readonlyOff {
		r.markReadonly(base)
	}
}

// elementDeclarationRefused asks the three axes a declaration of an element
// runs into, and reports the refusal where the dialect has one.
//
// Three questions rather than one, because they are three different things a
// declaration does to the variable and the panel does not answer them
// together: measured 2026-09-07, zsh 5.9.2 refuses all three and bash 5.3 and
// ksh93u+ take the integer attribute and the local scope, while the readonly
// one splits three ways and only two of the three are answered here.
func (r *Runner) elementDeclarationRefused(base, sub string, f declareFlags, shadows bool) bool {
	d := r.diag()
	if f.readonly && !f.readonlyOff {
		switch r.readonlyElementPolicy() {
		case ReadonlyElementRefused:
			r.refuseElementDeclaration(base, sub, d.ReadonlyElementRefusal)
			return true
		case ReadonlyElementWritten:
		default:
			return true
		}
	}
	if f.integer && !f.remove &&
		!r.ask(r.sem().SubscriptedOperandTakesTheIntegerAttribute,
			"an integer attribute on a declaration of one array element") {
		if !r.unspecified {
			r.refuseElementDeclaration(base, sub, d.IntegerElementRefusal)
		}
		return true
	}
	if shadows && !f.global && len(r.scopes) > 0 &&
		!r.ask(r.sem().SubscriptedOperandTakesALocalDeclaration,
			"a declaration of one array element making the array local") {
		if !r.unspecified {
			r.refuseElementDeclaration(base, sub, d.LocalElementRefusal)
		}
		return true
	}
	return false
}

// refuseElementDeclaration reports the refusal and ends the script.
//
// Fatal, in the one shell that has these: measured, `readonly a[1]=v` and
// `typeset -r a[1]=v` each print their line and nothing after them runs, in a
// file and through `-c` alike. The complaint names the operand as it was
// written rather than the base, which is the opposite of what a readonly
// refusal names — a refusal to *create* is about the element.
func (r *Runner) refuseElementDeclaration(base, sub, wording string) {
	r.diagf("%s\n", Wording(wording, "%[1]s[%[2]s]: cannot declare an array element", base, sub))
	// The status is the dialect's own answer for a fatal error rather than a
	// number of this refusal's: setFatalStatus is what fatalQuiet consults,
	// and a status set in front of it was simply overwritten.
	r.fatalQuiet()
}

// readonlyElementPolicy is the dialect's answer for a declaration that would
// freeze the array its operand names an element of.
func (r *Runner) readonlyElementPolicy() ReadonlyElementPolicy {
	p := r.sem().ReadonlyElement
	if p == ReadonlyElementUnspecified {
		r.diagf("%s\n", r.unanswered("a declaration of one array element freezing the whole array"))
		r.status = 2
		r.unspecified = true
	}
	return p
}

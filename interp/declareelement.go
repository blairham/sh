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
	// Before the attributes land, because whether the table letter on *this*
	// command reaches *this* command's subscript is a dialect's answer and
	// not an ordering the engine may pick — see
	// Semantics.TableLetterReachesItsOwnOperandsSubscript.
	tableBefore := r.assocDeclared(base)
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
	key, evaluated, isKey := r.subscriptedOperandKey(base, sub, tableBefore)
	switch {
	case r.unspecified:
		return
	case isKey && !evaluated:
		r.setAssocElem(base, key, value)
	case isKey:
		// A table whose letter arrived too late to be read: the subscript was
		// evaluated as an expression and the number it came to is the key.
		idx, err := r.subscriptValue(sub)
		if err != nil {
			r.fatal("%s\n", r.subscriptFailure(sub, err))
			return
		}
		r.setAssocElem(base, itoa(idx), value)
	default:
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
	// Ahead of the other three, which is measured rather than arbitrary: in
	// the one shell that refuses, `typeset -rA m[k]=v` answers with this
	// sentence and not the readonly one. The guard is what keeps the three
	// from ordering into a cycle — see
	// Semantics.SubscriptedOperandTakesTheContainerAttribute.
	if (f.array || f.assoc) && !f.remove && !r.typeLetterTakesTheCompoundLetter(f) &&
		!r.ask(r.sem().SubscriptedOperandTakesTheContainerAttribute,
			"a container attribute on a declaration of one array element") {
		if !r.unspecified {
			r.refuseElementDeclaration(base, sub, d.ContainerElementRefusal)
		}
		return true
	}
	if r.unspecified {
		return true
	}
	if f.readonly && !f.readonlyOff {
		switch r.readonlyElementPolicy() {
		case ReadonlyElementRefused:
			r.refuseElementDeclaration(base, sub, d.ReadonlyElementRefusal)
			return true
		case ReadonlyElementFrozenFirst:
			r.freezeBeforeTheElementWrite(base, f)
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

// subscriptedOperandKey is the key a subscripted operand's element goes under,
// where the base names a table — the text between the brackets, or the number
// the text evaluates to.
//
// A table declared *earlier* takes the key everywhere: `typeset -A m; typeset
// m[k]=v` stores under `k` in bash and ksh93 alike. The question is only about
// the letter written on the same command as the operand, and there the two
// part — measured 2026-09-12 on bash 5.3.15 and ksh93u+ 2012-08-01:
//
//	typeset -A m[k]=v      bash declare -A m=([k]="v")   ksh93 typeset -A m=([0]=v)
//	k=7; typeset -A m[k]=v bash declare -A m=([k]="v")   ksh93 typeset -A m=([7]=v)
//	typeset -A m[1+1]=v    bash declare -A m=([1+1]="v") ksh93 typeset -A m=([2]=v)
//
// #1380's re-measurement read the first row as ksh93 *discarding* the
// subscript and storing under `0`. The second and third rows say otherwise and
// cannot agree by accident: `0` was the value of the unset name `k`, and with
// `k=7` the key is `7`. The subscript is evaluated because the letter has not
// landed yet, which is the same ordering answer ReadonlyElement records for
// the freeze and not a rule about tables at all.
//
// Asked only where the letter is what made the difference. A base that was
// already a table, and a base that is not one either way, raise no question
// between the two readings.
// The three results are the key, whether it had to be *evaluated* to become
// one, and whether the base is a table at all.
func (r *Runner) subscriptedOperandKey(base, sub string, tableBefore bool) (key string, evaluated, isKey bool) {
	if tableBefore {
		return sub, false, true
	}
	if !r.assocDeclared(base) {
		return "", false, false
	}
	if r.ask(r.sem().TableLetterReachesItsOwnOperandsSubscript,
		"a table letter reaching the subscript of an operand on its own command") {
		return sub, false, true
	}
	// Still a table — the letter did land — so the element goes in it under
	// the number rather than beside it in an indexed array the name does not
	// have. ksh93 lists `typeset -A m=([7]=v)` for `k=7; typeset -A m[k]=v`,
	// which is the table holding the key `7`.
	return "", true, !r.unspecified
}

// freezeBeforeTheElementWrite is ReadonlyElementFrozenFirst: the container the
// letters name is declared, the freeze lands on it, and the element write is
// then lost to the freeze the same declaration has just applied.
//
// The order is the whole of it. Every other declaration here writes and then
// freezes, which is why this cannot be reached by moving a line: the write has
// to be *given up* rather than deferred, and the container still has to exist
// afterwards.
//
// f.array unless a table was asked for, because bash makes a subscripted
// operand's base an indexed array whether or not a container letter was
// written — `typeset -r a[1]=v` lists `declare -ar a=()` — and promotes a
// scalar it finds into element 0 rather than discarding it.
//
// Reported through the store's refusal rather than the builtin's, and at
// status 0: `typeset -r a[1]=v` says `a: readonly variable` where an
// already-frozen `typeset a[1]=v` says `typeset: a: readonly variable` at 1.
func (r *Runner) freezeBeforeTheElementWrite(base string, f declareFlags) {
	if !f.assoc {
		f.array = true
	}
	r.applyAttributes(base, f)
	r.markDeclaredCompound(base, false, f, true)
	if r.unspecified {
		return
	}
	r.markReadonly(base)
	outer := r.inBuiltin
	r.inBuiltin = ""
	defer func() { r.inBuiltin = outer }()
	r.diagf("%s\n", Wording(r.diag().ReadonlyVariable, "%s: readonly variable", base))
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

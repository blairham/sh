// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A call whose body reads past the locals of the calls below it.
//
// Every shell in the panel but one scopes a declaration **dynamically**: the
// name a function declared local is the name every function it calls sees.
// ksh93 scopes a `function`-form body statically — `function caller { typeset
// v=local; callee; }` leaves `callee` reading the shell's own `v` — and there
// is no second store to read it out of here, because a local in this
// interpreter is a save-and-restore over one table of names. So the seal is
// built the way the shell's own answer describes it: the enclosing calls'
// declarations are put aside for the duration and put back at the return.
//
// See Semantics.CallerLocalsReachTheCallee for the measurement.
//
// It is a **swap and not a copy**, which is the half a value-only reading
// would get wrong: a name the callee assigns is the shell's own name, so what
// the callee wrote has to be left in the enclosing scope's saved copy rather
// than thrown away with the seal. Measured, `function callee { v=written; };
// function caller { typeset v=local; callee; printf '[%s]' "$v"; }; v=global;
// caller; printf 'g=[%s]' "$v"` writes `[local]g=[written]` in ksh93 — the
// caller's local untouched and the global changed.
//
// **What the seal covers is stated rather than assumed.** It is the value in
// each of the three tables, whether `unset` had hidden the name, and every
// attribute a declaration displaces — measured one at a time, because a seal
// over the scalar alone would answer the headline case and leave a caller's
// `typeset -i` deciding how the shell's own name is written. Two stores are
// deliberately outside it: a shell-*produced* parameter a declaration hid
// (Semantics.DeclareHideInScopeLetter, a letter ksh93 does not have) and the message
// such a parameter's producer was last handed. No column can put a question
// to either, so what is here is what was measured rather than what was
// convenient to reach.

// staticBinding is what one name carries: its value in each of the three
// tables, and every attribute that travels with it.
//
// Each half is flagged rather than assumed present, because the two sources
// differ. Read off the runner every field is there; read off a scope, only
// the maps that scope really shadowed are — and a missing entry means "this
// scope said nothing about that", which is not the same as "there was
// nothing". Installing a zero for one would take the export attribute off a
// name the seal was only meant to read past.
type staticBinding struct {
	value         string
	array         Array
	assoc         AssocArray
	attrs         nameAttributes
	assignedTo    string
	valueExists   bool
	arrayExists   bool
	assocExists   bool
	removed       bool
	readonly      bool
	hideInScope   bool
	exported      bool
	assignedSaid  bool
	exportedSaid  bool
	declaredOnly  bool
	heldAnElement bool
	scalarHeld    bool

	hasValue    bool
	hasArray    bool
	hasAssoc    bool
	hasRemoved  bool
	hasReadonly bool
	hasHide     bool
	hasAttrs    bool
	hasAssigned bool
	hasExported bool
	hasCompound bool
}

// sealedName is one name put aside for the duration of a sealed call.
type sealedName struct {
	// holder is the enclosing scope whose saved copy *is* the shell's own
	// name for as long as the seal stands, so that what the sealed body
	// writes is left there rather than thrown away.
	//
	// The **innermost** enclosing scope that shadowed the name, which reads
	// backwards and is right: under this dialect every call that takes a
	// scope is itself sealed, so each one shadowed the shell's own name
	// rather than its caller's local, and the newest such copy is the
	// freshest. The caller's locals are not in the scopes at all — they are
	// in held, one frame per call.
	holder *scope
	// held is what the runner carried when the seal went up: the caller's
	// local, which comes back at the return.
	held staticBinding
}

// captureBinding reads everything the runner holds about a name.
func (r *Runner) captureBinding(name string) staticBinding {
	b := staticBinding{
		hasValue: true, hasArray: true, hasAssoc: true, hasRemoved: true,
		hasReadonly: true, hasHide: true, hasAttrs: true, hasAssigned: true,
		hasExported: true, hasCompound: true,
	}
	b.value, b.valueExists = r.varsLookup(name)
	if a, ok := r.arraysLookup(name); ok {
		// Cloned for the reason shadow clones one: an Array is a map, so
		// keeping the value would keep a reference to the table the callee
		// is about to write into and putting it back would put the writes
		// back with it.
		b.array, b.arrayExists = a.clone(), true
	}
	if m, ok := r.assocsLookup(name); ok {
		b.assoc, b.assocExists = m.clone(), true
	}
	b.removed = r.removed[name]
	b.readonly = r.attrHas(name, attrReadonly)
	b.hideInScope = r.attrHas(name, attrHideInScope)
	b.attrs = r.captureAttributes(name)
	b.assignedTo, b.assignedSaid = r.assigned[name]
	b.exported, b.exportedSaid = r.exported[name]
	b.declaredOnly = r.attrHas(name, attrDeclaredOnlyCompound)
	b.heldAnElement = r.attrHas(name, attrHeldAnElement)
	b.scalarHeld = r.attrHas(name, attrScalarUnderTheArrayLetter)
	return b
}

// bindingFromScope reads what a scope saved about a name, saying for each
// table whether that scope saved anything at all.
func bindingFromScope(sc *scope, name string) staticBinding {
	var b staticBinding
	rec := sc.record(name)
	if rec == nil {
		return b
	}
	b.value, b.hasValue, b.valueExists = rec.value, rec.holds(recValue), rec.bit(recValue)
	b.array, b.hasArray, b.arrayExists = rec.array, rec.holds(recArray), rec.bit(recArray)
	b.assoc, b.hasAssoc, b.assocExists = rec.assoc, rec.holds(recAssoc), rec.bit(recAssoc)
	b.removed, b.hasRemoved = rec.bit(recRemovedBefore), rec.holds(recRemovedBefore)
	b.readonly, b.hasReadonly = rec.bit(recSavedReadonly), rec.holds(recSavedReadonly)
	b.hideInScope, b.hasHide = rec.bit(recSavedHideInScope), rec.holds(recSavedHideInScope)
	b.attrs, b.hasAttrs = rec.attrs, rec.holds(recAttrs)
	b.assignedTo, b.hasAssigned, b.assignedSaid = rec.assigned, rec.holds(recAssigned), rec.bit(recAssigned)
	b.exported, b.hasExported, b.exportedSaid = rec.exportedOn, rec.holds(recExported), rec.bit(recExported)
	b.declaredOnly, b.hasCompound = rec.bit(recDeclaredOnlyBefore), rec.holds(recDeclaredOnlyBefore)
	b.heldAnElement = rec.holds(flagHeldAnElementBefore)
	b.scalarHeld = rec.holds(flagScalarHeldBefore)
	return b
}

// installBinding writes a binding back onto the runner, touching only the
// tables it has something to say about.
func (r *Runner) installBinding(name string, b staticBinding) {
	if b.hasValue {
		if b.valueExists {
			r.varsPut(name, b.value)
		} else {
			r.varsDel(name)
		}
		r.ignoredNamesRestored(name)
	}
	if b.hasArray {
		if b.arrayExists {
			r.arraysPut(name, b.array)
		} else {
			r.arraysDel(name)
		}
	}
	if b.hasAssoc {
		if b.assocExists {
			// A copy, because the binding keeps the table it came from: put
			// back as it is, one map would be both the scope's and a live
			// table a subshell may come to share. See sharedtable.go.
			r.ownAssocs()
			r.assocsPut(name, b.assoc.clone())
		} else {
			r.dropAssocTable(name)
		}
	}
	if b.hasCompound {
		if b.declaredOnly {
			r.compoundDeclaredOnly(name)
		} else {
			r.compoundWasAssigned(name)
		}
		r.attrPut(name, attrHeldAnElement, b.heldAnElement)
		r.attrPut(name, attrScalarUnderTheArrayLetter, b.scalarHeld)
	}
	if b.hasRemoved {
		setBool(&r.removed, name, b.removed)
	}
	if b.hasReadonly {
		r.attrPut(name, attrReadonly, b.readonly)
	}
	if b.hasHide {
		r.attrPut(name, attrHideInScope, b.hideInScope)
	}
	if b.hasAttrs {
		r.restoreAttributes(name, b.attrs)
	}
	if b.hasAssigned {
		if b.assignedSaid {
			if r.assigned == nil {
				r.assigned = map[string]string{}
			}
			r.assigned[name] = b.assignedTo
		} else {
			delete(r.assigned, name)
		}
	}
	if b.hasExported {
		if b.exportedSaid {
			r.exported[name] = b.exported
		} else {
			delete(r.exported, name)
		}
	}
}

// writeBindingToScope leaves what the runner now holds in the scope's own
// saved copy, so that the enclosing call gets the callee's writes rather than
// the value it had when the seal went up.
//
// Only the tables that scope had already saved are written, for the reason
// bindingFromScope flags them: an entry invented here would make the scope
// claim to have shadowed something it never did, and its return would then
// undo a name it was never asked about.
func writeBindingToScope(sc *scope, name string, b staticBinding) {
	rec := sc.record(name)
	if rec == nil {
		return
	}
	if rec.holds(recValue) {
		rec.put(recValue, b.valueExists)
		rec.value = b.value
	}
	if rec.holds(recArray) {
		rec.put(recArray, b.arrayExists)
		rec.array = b.array
	}
	if rec.holds(recAssoc) {
		rec.put(recAssoc, b.assocExists)
		rec.assoc = b.assoc
	}
	if rec.holds(recRemovedBefore) {
		rec.put(recRemovedBefore, b.removed)
	}
	if rec.holds(recSavedReadonly) {
		rec.put(recSavedReadonly, b.readonly)
	}
	if rec.holds(recSavedHideInScope) {
		rec.put(recSavedHideInScope, b.hideInScope)
	}
	if rec.holds(recAttrs) {
		rec.attrs = b.attrs
	}
	if rec.holds(recAssigned) {
		rec.put(recAssigned, b.assignedSaid)
		rec.assigned = b.assignedTo
	}
	if rec.holds(recExported) {
		rec.put(recExported, b.exportedSaid)
		rec.exportedOn = b.exported
	}
	if rec.holds(recDeclaredOnlyBefore) {
		rec.put(recDeclaredOnlyBefore, b.declaredOnly)
		rec.setFlag(flagHeldAnElementBefore, b.heldAnElement)
	}
}

// keywordGatesALocalScope reports whether only a `function`-word body gets a
// scope of its own here.
//
// **Read rather than asked**, which is the rare shape and wants its reason.
// It chooses which calls the axis below is even about, and the axis it reads
// is answered where its two answers are observable — shadowTypeset asks it
// over a declaration that is either local or not. Asking it here as well
// would refuse a call whose body may never declare anything, over a question
// that call never puts.
func (r *Runner) keywordGatesALocalScope() bool {
	return r.sem().TypesetLocalNeedsKeywordFunction == Yes
}

// sealCallerLocals puts the enclosing calls' declarations aside where this
// dialect's function bodies do not see them.
//
// Asked at the disagreement and nowhere else. A call with nothing shadowed
// below it has no question to put — every column answers it the same way —
// and a call that takes no variable scope of its own is not a boundary at
// all: measured, ksh93's own POSIX-form function *does* see the caller's
// local, because a `typeset` there was never local to begin with.
func (r *Runner) sealCallerLocals(sc *scope) {
	if len(r.scopes) < 2 {
		return
	}
	if r.sem().CallerLocalsReachTheCallee == Yes {
		// The common answer, taken before the walk below rather than after
		// it. A read and not an ask, and it decides nothing: the *ask* is
		// still where the disagreement is, and this only spares the shells
		// that hand the caller's local down a scope walk on every call.
		return
	}
	if !sc.keyword && r.keywordGatesALocalScope() {
		return
	}
	holders := r.callerShadowHolders()
	if len(holders) == 0 {
		return
	}
	if r.ask(r.sem().CallerLocalsReachTheCallee,
		"a function body seeing a name one of its callers declared local") {
		return
	}
	sc.sealed = make(map[string]sealedName, len(holders))
	for name, holder := range holders {
		sc.sealed[name] = sealedName{holder: holder, held: r.captureBinding(name)}
		r.installBinding(name, bindingFromScope(holder, name))
	}
}

// unsealCallerLocals is the return: each caller gets its declaration back, and
// the shell's own name keeps whatever the sealed body wrote to it.
func (r *Runner) unsealCallerLocals(sc *scope) {
	for name, sn := range sc.sealed {
		writeBindingToScope(sn.holder, name, r.captureBinding(name))
		r.installBinding(name, sn.held)
	}
	sc.sealed = nil
}

// callerShadowHolders is, per name any call below this one declared local,
// the innermost such call's scope.
func (r *Runner) callerShadowHolders() map[string]*scope {
	var holders map[string]*scope
	for i := len(r.scopes) - 2; i >= 0; i-- {
		for _, rec := range r.scopes[i].records {
			if !rec.holds(recValue) {
				continue
			}
			name := rec.name
			if _, taken := holders[name]; taken {
				continue
			}
			if holders == nil {
				holders = map[string]*scope{}
			}
			holders[name] = r.scopes[i]
		}
	}
	return holders
}

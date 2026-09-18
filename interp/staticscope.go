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
	b.value, b.valueExists = r.Vars[name]
	if a, ok := r.Arrays[name]; ok {
		// Cloned for the reason shadow clones one: an Array is a map, so
		// keeping the value would keep a reference to the table the callee
		// is about to write into and putting it back would put the writes
		// back with it.
		b.array, b.arrayExists = a.clone(), true
	}
	if m, ok := r.AssocArrays[name]; ok {
		b.assoc, b.assocExists = m.clone(), true
	}
	b.removed = r.removed[name]
	b.readonly = r.readonly[name]
	b.hideInScope = r.hideInScope[name]
	b.attrs = r.captureAttributes(name)
	b.assignedTo, b.assignedSaid = r.assigned[name]
	b.exported, b.exportedSaid = r.exported[name]
	b.declaredOnly = r.declaredOnlyCompound[name]
	b.heldAnElement = r.compoundHeldAnElement[name]
	return b
}

// bindingFromScope reads what a scope saved about a name, saying for each
// table whether that scope saved anything at all.
func bindingFromScope(sc *scope, name string) staticBinding {
	var b staticBinding
	b.value, b.hasValue = sc.saved[name]
	if b.hasValue {
		b.valueExists = sc.existed[name]
	}
	b.array, b.hasArray = sc.savedArrays[name]
	if b.hasArray {
		b.arrayExists = sc.arrayExisted[name]
	}
	b.assoc, b.hasAssoc = sc.savedAssoc[name]
	if b.hasAssoc {
		b.assocExists = sc.assocExisted[name]
	}
	b.removed, b.hasRemoved = sc.removedBefore[name]
	b.readonly, b.hasReadonly = sc.savedReadonly[name]
	b.hideInScope, b.hasHide = sc.savedHideInScope[name]
	b.attrs, b.hasAttrs = sc.savedAttrs[name]
	b.assignedTo, b.hasAssigned = sc.savedAssigned[name]
	if b.hasAssigned {
		b.assignedSaid = sc.assignedSpoken[name]
	}
	b.exported, b.hasExported = sc.savedExported[name]
	if b.hasExported {
		b.exportedSaid = sc.exportedSpoken[name]
	}
	b.declaredOnly, b.hasCompound = sc.declaredOnlyBefore[name]
	b.heldAnElement = sc.heldAnElementBefore[name]
	return b
}

// installBinding writes a binding back onto the runner, touching only the
// tables it has something to say about.
func (r *Runner) installBinding(name string, b staticBinding) {
	if b.hasValue {
		if b.valueExists {
			r.Vars[name] = b.value
		} else {
			delete(r.Vars, name)
		}
		r.ignoredNamesRestored(name)
	}
	if b.hasArray {
		if b.arrayExists {
			r.Arrays[name] = b.array
		} else {
			delete(r.Arrays, name)
		}
	}
	if b.hasAssoc {
		if b.assocExists {
			r.AssocArrays[name] = b.assoc
		} else {
			delete(r.AssocArrays, name)
		}
	}
	if b.hasCompound {
		if b.declaredOnly {
			r.compoundDeclaredOnly(name)
		} else {
			r.compoundWasAssigned(name)
		}
		setBool(&r.compoundHeldAnElement, name, b.heldAnElement)
	}
	if b.hasRemoved {
		setBool(&r.removed, name, b.removed)
	}
	if b.hasReadonly {
		setBool(&r.readonly, name, b.readonly)
	}
	if b.hasHide {
		setBool(&r.hideInScope, name, b.hideInScope)
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
	if _, ok := sc.saved[name]; ok {
		sc.saved[name], sc.existed[name] = b.value, b.valueExists
	}
	if _, ok := sc.savedArrays[name]; ok {
		sc.savedArrays[name], sc.arrayExisted[name] = b.array, b.arrayExists
	}
	if _, ok := sc.savedAssoc[name]; ok {
		sc.savedAssoc[name], sc.assocExisted[name] = b.assoc, b.assocExists
	}
	if _, ok := sc.removedBefore[name]; ok {
		sc.removedBefore[name] = b.removed
	}
	if _, ok := sc.savedReadonly[name]; ok {
		sc.savedReadonly[name] = b.readonly
	}
	if _, ok := sc.savedHideInScope[name]; ok {
		sc.savedHideInScope[name] = b.hideInScope
	}
	if _, ok := sc.savedAttrs[name]; ok {
		sc.savedAttrs[name] = b.attrs
	}
	if _, ok := sc.savedAssigned[name]; ok {
		sc.savedAssigned[name], sc.assignedSpoken[name] = b.assignedTo, b.assignedSaid
	}
	if _, ok := sc.savedExported[name]; ok {
		sc.savedExported[name], sc.exportedSpoken[name] = b.exported, b.exportedSaid
	}
	if _, ok := sc.declaredOnlyBefore[name]; ok {
		sc.declaredOnlyBefore[name] = b.declaredOnly
		setBool(&sc.heldAnElementBefore, name, b.heldAnElement)
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
		for name := range r.scopes[i].saved {
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

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// shadowRecord is everything one scope keeps about one name it shadowed: what
// the declaration displaced, and the attributes it took off, so that the
// scope's exit can put each of them back.
//
// One record per name rather than a table per kind of record. The scope used
// to keep about twenty maps keyed by name — the value, whether it existed, the
// frozen attribute, the export record and a dozen more — so a single `local a`
// was some twenty map writes at the shadow, twenty lookups and twenty deletes
// at the return, and a walk over fourteen tables to list the names to put
// back. That was most of what a declaration cost once its allocations were
// gone (#6373). A record is one lookup and one set of field writes, and the
// list of names is the list of records.
//
// Which records are present is a bit set, because absent and false mean
// different things for most of them: see recordKind. A record whose bits are
// all clear is one whose name has been put back — restoreShadowedName clears
// what it restores rather than deleting the record, so a name shadowed again
// in the same scope reuses it.
type shadowRecord struct {
	name string
	// has says which records are present, and which flags are on.
	has recordKind
	// bits is the boolean each present record holds: whether the value,
	// array or table existed, what the frozen attribute was, and so on, under
	// the same bit as the record itself. See recordKind for which kinds keep
	// their answer here.
	bits recordKind
	// exportedOn is the export record's value where bits holds whether it had
	// been spoken about at all — the one record with two booleans.
	exportedOn bool

	value          string
	assigned       string
	exportedShadow string
	array          Array
	assoc          AssocArray
	attrs          nameAttributes
	producer       *suspendedProducer
}

// recordKind names one record a scope keeps about a name. The constants are
// bits, so a record's presence and its boolean answer are each one word.
type recordKind uint32

const (
	// recValue is the scalar a declaration displaced, and its bit whether
	// the name held one at all.
	recValue recordKind = 1 << iota
	// recArray is the same for arrays, which are a second table: shadowing
	// only Vars left `f() { local a; a=(x y); }` setting a *global* array,
	// because nothing had saved the array of that name to put back. The bit
	// is whether the name was an array.
	recArray
	// recAssoc shadows the associative table the same way, attribute and
	// all: what comes back on exit is whether the name was associative as
	// much as what it held.
	recAssoc
	// recRemovedBefore is whether `unset` had already hidden the name when
	// it was shadowed, so a hiding `local` can be undone without resurrecting
	// an environment value the script had taken away — or forgetting one it
	// had not.
	recRemovedBefore
	// recDeclaredOnlyBefore is whether the name's compound value had come
	// from a declaration alone when it was shadowed, so a `local -A m` that
	// empties nothing does not leave the caller's table reading as though a
	// declaration had just made it. Saved beside the two compound tables and
	// put back with them. See compounddeclaredonly.go.
	recDeclaredOnlyBefore
	// recCompoundMarkBefore is whether a shadowed name was a compound
	// variable when the declaration displaced it. Saved beside the three
	// value records because the mark *is* the name's kind — a name holding
	// the fourth kind holds nothing in any of them — so restoring the three
	// alone left the caller's `c` reading as a compound with no members, or
	// as an ordinary name that had been one. Absent means this scope never
	// shadowed the name and **false** that it shadowed one that was not a
	// compound, which is what takes back a mark the call itself added. See
	// compoundlocal.go.
	recCompoundMarkBefore
	// recSavedReadonly is whether a shadowed name was frozen when the
	// declaration displaced it. Taking the attribute off is a change to the
	// runner's record and has to be put back like the value — otherwise the
	// outer name comes back assignable, which is a frozen name quietly
	// thawed by a function call.
	//
	// Written for *every* name a declaration shadows and not only for the
	// ones that arrived frozen, because the entry has two jobs and the
	// second one needs the false. A `false` here says the shell had nothing
	// frozen under this name when the call took it, so anything frozen by
	// the time the call returns was frozen *by the call* and goes away with
	// it: `f() { local -r y=1; }; f; y=2` leaves y writable in bash, ksh93
	// and zsh alike, and recording only the true left the caller's y frozen
	// for the rest of the script.
	//
	// So absent and false do not mean the same thing here, though they do in
	// attrReadonly: absent means this scope never shadowed the name at all.
	recSavedReadonly
	// recSavedHideInScope is the hide-in-scope attribute a shadowed name
	// carried when the declaration displaced it, saved for the same two
	// reasons recSavedReadonly is: the outer name gets its own attribute
	// back, and one this call added goes away with the call. Absent and false
	// do not mean the same thing here either — absent means this scope never
	// shadowed the name. See hideinscope.go.
	recSavedHideInScope
	// recHiddenShadow is whether the shadow this scope took over a name is a
	// *hidden* one — an ordinary parameter that merely happens to be spelled
	// like one of the shell's own, rather than a second view of it. Written
	// for every name the scope shadows, because absent and false are
	// different questions here too: absent means this scope never shadowed
	// the name, and the innermost scope that did is the one that answers.
	//
	// Separate from recSavedHideInScope, which is a *value* being kept for
	// the return, because the two disagree the moment either `-h` or `+h` is
	// written on the declaration — and separate from attrHideInScope, which
	// after the shadow describes the fresh binding and no longer the outer
	// one that governed it. See hideinscope.go.
	recHiddenShadow
	// recProducer is what a hidden shadow took out of the produced tables,
	// so that the name reads and writes as an ordinary parameter for as long
	// as the shadow stands and is the shell's own again on return. See
	// hideinscope.go.
	recProducer
	// recAttrs is every other attribute a shadowed name carried — integer
	// and its base, float and its precision, the two case letters, unique
	// and hidden. Saved and taken off at the shadow and put back on return,
	// for the two reasons recSavedReadonly is: a local is a fresh binding
	// that inherits none of them, and one this call added goes away with the
	// call. Absent means this scope never shadowed the name; a zero value
	// means it shadowed one that carried nothing. See localattributes.go.
	recAttrs
	// recAssigned is what a *produced* parameter was last assigned when the
	// declaration displaced it, and its bit is whether it had been assigned
	// at all — the same tri-state recExported keeps, and for the same reason:
	// "nothing was ever assigned" is a state, and a single value could not
	// tell it from "assigned the empty string".
	//
	// A produced parameter's assignment does not live in Vars, so shadowing
	// Vars alone shadowed nothing. `f() { local -i SECONDS=1024; }; f` left
	// SECONDS reading 1024 for the rest of the session, where zsh 5.9.2 has
	// it counting from the call again — and powerlevel10k measures its prompt
	// inside exactly that shape, `local -i COLUMNS=1024`, so the width it
	// forced for the measurement escaped into the session and every prompt
	// afterwards was drawn 1024 columns wide (#2107).
	recAssigned
	// recExported is the export attribute a shadowed name had, for the
	// dialects where a local does not inherit it. Taking the attribute off is
	// a change to the runner's record and has to be put back like the value,
	// and the record is a tri-state — so what was there is two booleans
	// rather than one: whether it had been spoken about at all, which is the
	// bit, and what it said, which is exportedOn.
	recExported
	// recExportedShadow is what an exported name held when this scope's
	// declaration took it out of view — the value a child is told for a
	// local that inherited the attribute and was left with none of its own.
	// See hiddenExports. Not put back by restoreShadowedName: it describes
	// the binding the scope hid and dies with the scope.
	recExportedShadow

	// The flags below are on or absent, never false: present is the answer.

	// flagHeldAnElementBefore is whether the name's indexed array had ever
	// held an element when it was shadowed, so a local array that is emptied
	// inside a call does not leave the caller's name reading as one that has
	// lost its elements. See attrHeldAnElement.
	flagHeldAnElementBefore
	// flagScalarHeldBefore is the same save for
	// attrScalarUnderTheArrayLetter.
	flagScalarHeldBefore
	// flagMemberNamespace says this scope shadowed the name's **member**
	// namespace — every name under `c.` displaced along with `c` itself. It
	// is read on the write path as well as on the exit: a member the body
	// creates has nothing saved for it, and this is what says the name
	// belongs to the call. See compoundlocal.go.
	flagMemberNamespace
	// flagTieMirrorOnly says this scope saved the name **only** because the
	// other half of one of the shell's own ties was declared: `local PATH`
	// displaces `path` with it, because the tie is a property of the name and
	// a local of one half has to be a local of the pair.
	//
	// The flag exists so the *word* can tell that shadow from a declaration
	// of the name itself. They are the same displacement and they describe
	// differently — measured, `f(){ typeset PATH; print ${(t)path} }` is
	// `array-tied-special` in the reference, with no `local` in it, where
	// `f(){ typeset path; … }` is `array-local-tied-special`. The fact was
	// already written down in tielocal.go's own table and nothing had asked
	// the word about it (#4875).
	//
	// Cleared by [Runner.shadow] whenever the name is declared directly,
	// outside its freshness check and for the reason `privateShadowTaken` is
	// outside it: a second declaration naming this name is a declaration of
	// it, whatever displaced the cell first.
	flagTieMirrorOnly
)

// restoredRecords are the records a scope's exit puts back, and so the ones
// whose presence puts a name on the list shadowedNames returns. The others
// are kept beside them without being restored: the existed halves ride on
// their value's bit, the two element flags on recDeclaredOnlyBefore, and the
// tie mark and the exported shadow describe the binding rather than the name
// it displaced.
const restoredRecords = recValue | recArray | recAssoc | recRemovedBefore |
	recDeclaredOnlyBefore | recCompoundMarkBefore | recSavedReadonly |
	recSavedHideInScope | recHiddenShadow | recProducer | recAttrs |
	recAssigned | recExported | flagMemberNamespace

// freshShadowRecords are the records Runner.shadow writes for every name it
// takes fresh, each of them whether or not there was anything to save, and
// compoundShadowRecords the three it writes beside them once per name per
// scope. Named so that the shadow of a name with nothing to save — see
// emptyshadow.go — is the same set of records and cannot fall behind the
// shadow it stands in for.
const (
	freshShadowRecords = recValue | recRemovedBefore | recSavedReadonly | recSavedHideInScope |
		recHiddenShadow | recAttrs | recExported | recAssigned
	compoundShadowRecords = recArray | recAssoc | recDeclaredOnlyBefore
)

// recordIndexAt is how many records a scope holds before it keeps an index of
// them by name. Below it a lookup compares names along the list, which for
// the handful a function body declares is cheaper than hashing one; past it
// a long list would make every lookup a walk.
const recordIndexAt = 8

// holds reports whether the record carries kind k.
func (rec *shadowRecord) holds(k recordKind) bool { return rec.has&k != 0 }

// bit is the boolean kind k holds, false where the record is absent.
func (rec *shadowRecord) bit(k recordKind) bool { return rec.bits&k != 0 }

// put records kind k with the boolean v.
func (rec *shadowRecord) put(k recordKind, v bool) {
	rec.has |= k
	if v {
		rec.bits |= k
	} else {
		rec.bits &^= k
	}
}

// setFlag turns flag k on or off.
func (rec *shadowRecord) setFlag(k recordKind, on bool) {
	if on {
		rec.has |= k
	} else {
		rec.has &^= k
	}
}

// drop forgets kind k, and lets go of whatever it held.
func (rec *shadowRecord) drop(k recordKind) {
	rec.has &^= k
	rec.bits &^= k
	switch k {
	case recValue:
		rec.value = ""
	case recArray:
		rec.array = nil
	case recAssoc:
		rec.assoc = nil
	case recProducer:
		rec.producer = nil
	case recAttrs:
		rec.attrs = nameAttributes{}
	case recAssigned:
		rec.assigned = ""
	case recExported:
		rec.exportedOn = false
	case recExportedShadow:
		rec.exportedShadow = ""
	}
}

// record is the scope's record of a name, or nil where it has none.
func (sc *scope) record(name string) *shadowRecord {
	if sc.recordIndex != nil {
		return sc.recordIndex[name]
	}
	for _, rec := range sc.records {
		if sameName(rec.name, name) {
			return rec
		}
	}
	return nil
}

// recordFor is the scope's record of a name, made if it has none.
//
// A record is never moved once made — the list holds pointers — so one held
// across a call that shadows other names, as shadow holds its own across the
// members of a compound, stays the record of its name.
func (sc *scope) recordFor(name string) *shadowRecord {
	if rec := sc.record(name); rec != nil {
		return rec
	}
	var rec *shadowRecord
	if sc.spares != nil {
		rec = sc.spares.takeRecord()
		if sc.records == nil {
			sc.records = sc.spares.takeRecordList()
		}
	} else {
		rec = new(shadowRecord)
	}
	rec.name = name
	sc.records = append(sc.records, rec)
	switch {
	case sc.recordIndex != nil:
		sc.recordIndex[name] = rec
	case len(sc.records) > recordIndexAt:
		sc.recordIndex = make(map[string]*shadowRecord, 2*len(sc.records))
		for _, have := range sc.records {
			sc.recordIndex[have.name] = have
		}
	}
	return rec
}

// holds reports whether the scope has kind k recorded for the name.
func (sc *scope) holds(k recordKind, name string) bool {
	rec := sc.record(name)
	return rec != nil && rec.has&k != 0
}

// boolRecord is the boolean kind k holds for the name, and whether the scope
// recorded one at all.
func (sc *scope) boolRecord(k recordKind, name string) (v, ok bool) {
	rec := sc.record(name)
	if rec == nil || rec.has&k == 0 {
		return false, false
	}
	return rec.bits&k != 0, true
}

// recordBit is the boolean kind k holds for the name, false where the scope
// recorded none — for the existed halves, whether the name held a value of
// that kind when it was shadowed.
func (sc *scope) recordBit(k recordKind, name string) bool {
	rec := sc.record(name)
	return rec != nil && rec.bits&k != 0
}

// putRecord records kind k for the name with the boolean v.
func (sc *scope) putRecord(k recordKind, name string, v bool) {
	sc.recordFor(name).put(k, v)
	sc.made |= k
}

// dropRecord forgets kind k for the name.
func (sc *scope) dropRecord(k recordKind, name string) {
	if rec := sc.record(name); rec != nil {
		rec.drop(k)
	}
}

// madeRecord reports whether this scope has ever recorded kind k for any
// name.
//
// A question about the scope rather than about a name, and three writers ask
// it: a cell written outside a declaration — a frame's store, `declare -g`, a
// reference landing on its own name — updates the removed-by-`unset` and
// export records only in a scope that already keeps them, and leaves a scope
// that never did as it was, so that the return does not start touching a
// table nothing in the call had moved.
func (sc *scope) madeRecord(k recordKind) bool { return sc.made&k != 0 }

// flag reports whether flag k is on for the name.
func (sc *scope) flag(k recordKind, name string) bool { return sc.holds(k, name) }

// setFlag turns flag k on or off for the name. Off makes no record for a
// name that has none.
func (sc *scope) setFlag(k recordKind, name string, on bool) {
	if on {
		sc.recordFor(name).has |= k
		sc.made |= k
		return
	}
	if rec := sc.record(name); rec != nil {
		rec.setFlag(k, false)
	}
}

// savedValue is the scalar the scope saved for the name, whether the name held
// one, and whether the scope saved one at all.
func (sc *scope) savedValue(name string) (value string, existed, ok bool) {
	rec := sc.record(name)
	if rec == nil || rec.has&recValue == 0 {
		return "", false, false
	}
	return rec.value, rec.bits&recValue != 0, true
}

// shadowsValue reports whether the scope saved a scalar for the name — the
// question "did this scope take the name", since every declaration saves one.
func (sc *scope) shadowsValue(name string) bool { return sc.holds(recValue, name) }

// saveValue records the scalar the name is to get back.
func (sc *scope) saveValue(name, value string, existed bool) {
	rec := sc.recordFor(name)
	rec.put(recValue, existed)
	rec.value = value
	sc.made |= recValue
}

// savedArray is savedValue for the indexed table.
func (sc *scope) savedArray(name string) (a Array, existed, ok bool) {
	rec := sc.record(name)
	if rec == nil || rec.has&recArray == 0 {
		return nil, false, false
	}
	return rec.array, rec.bits&recArray != 0, true
}

// saveArray is saveValue for the indexed table.
func (sc *scope) saveArray(name string, a Array, existed bool) {
	rec := sc.recordFor(name)
	rec.put(recArray, existed)
	rec.array = a
	sc.made |= recArray
}

// savedAssocTable is savedValue for the associative table.
func (sc *scope) savedAssocTable(name string) (m AssocArray, existed, ok bool) {
	rec := sc.record(name)
	if rec == nil || rec.has&recAssoc == 0 {
		return nil, false, false
	}
	return rec.assoc, rec.bits&recAssoc != 0, true
}

// saveAssocTable is saveValue for the associative table.
func (sc *scope) saveAssocTable(name string, m AssocArray, existed bool) {
	rec := sc.recordFor(name)
	rec.put(recAssoc, existed)
	rec.assoc = m
	sc.made |= recAssoc
}

// savedAttributes is what the scope saved of the name's other attributes.
func (sc *scope) savedAttributes(name string) (nameAttributes, bool) {
	rec := sc.record(name)
	if rec == nil || rec.has&recAttrs == 0 {
		return nameAttributes{}, false
	}
	return rec.attrs, true
}

// saveAttributes records the name's other attributes.
func (sc *scope) saveAttributes(name string, a nameAttributes) {
	rec := sc.recordFor(name)
	rec.has |= recAttrs
	rec.attrs = a
	sc.made |= recAttrs
}

// suspendedProducerOf is the producer a hidden shadow of the name took out of
// the produced tables.
func (sc *scope) suspendedProducerOf(name string) (suspendedProducer, bool) {
	rec := sc.record(name)
	if rec == nil || rec.has&recProducer == 0 {
		return suspendedProducer{}, false
	}
	return *rec.producer, true
}

// saveSuspendedProducer records the producer a hidden shadow took aside.
func (sc *scope) saveSuspendedProducer(name string, p suspendedProducer) {
	rec := sc.recordFor(name)
	rec.has |= recProducer
	rec.producer = &p
	sc.made |= recProducer
}

// saveExport records the name's export record.
func (sc *scope) saveExport(name string, on, spoken bool) {
	rec := sc.recordFor(name)
	rec.put(recExported, spoken)
	rec.exportedOn = on
	sc.made |= recExported
}

// saveExportedShadow records what an exported name held when the scope's
// declaration took it out of view.
func (sc *scope) saveExportedShadow(name, value string) {
	rec := sc.recordFor(name)
	rec.has |= recExportedShadow
	rec.exportedShadow = value
	sc.made |= recExportedShadow
}

// shadowedNames is every name this scope has something to put back for, taken
// before any of them is put back: restoreShadowedName clears the records as it
// goes, so the list has to be settled first.
//
// Appended to buf, which the caller owns: the unwind lends the runner's own
// list for the length of one return. See Runner.popScope.
func (sc *scope) shadowedNames(buf []string) []string {
	names := buf[:0]
	for _, rec := range sc.records {
		if rec.has&restoredRecords != 0 {
			names = append(names, rec.name)
		}
	}
	return names
}

// cloneRecords gives a clone's scope its own copy of every record, and of
// every container a record holds: the two tables are maps under the name, so
// copying the record alone would hand a subshell the parent's elements to
// write.
func (sc *scope) cloneRecords() {
	if sc.records == nil {
		sc.recordIndex = nil
		return
	}
	recs := make([]*shadowRecord, len(sc.records))
	for i, rec := range sc.records {
		c := *rec
		c.array = rec.array.clone()
		c.assoc = rec.assoc.clone()
		if rec.producer != nil {
			p := *rec.producer
			c.producer = &p
		}
		recs[i] = &c
	}
	sc.records = recs
	if sc.recordIndex != nil {
		sc.recordIndex = make(map[string]*shadowRecord, len(recs))
		for _, rec := range recs {
			sc.recordIndex[rec.name] = rec
		}
	}
}

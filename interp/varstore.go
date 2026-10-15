// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"iter"
	"maps"
	"math/bits"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
)

// The variable store: one cell per name, and the only file that touches it.
//
// A name can be several things at once. An indexed array keeps a copy of one
// element as a scalar, a produced parameter keeps what was assigned to it
// beside the producer, and a name declared an associative array can still
// hold the scalar it had before. So the store is not one table per kind but
// one record per name with a slot per kind, and a presence bit per slot:
//
//	scalar          what `$name` stores                     (was Vars)
//	array           an indexed array                        (was Arrays)
//	assoc           an associative array; present is the    (was AssocArrays)
//	                `declare -A` attribute
//	producer        a scalar produced when it is read       (was Dynamic)
//	arrayProducer   an array produced when it is read       (was DynamicArrays)
//	assocProducer   a keyed table produced when it is read  (was DynamicAssocs)
//
// Those were six exported maps on Runner until #6353, written directly in ~400
// places. A read of one scalar asked about a dozen of them in a fixed order
// (Runner.storedValue), and nothing could answer that in one lookup, because
// no record owned the name: a delete from one table and an insert into
// another leave every length the same, so no index over them could be kept
// sound. The cell is that record. The first step routed every site through
// the methods below with the maps unchanged; this is the second, which
// changes only this file.
//
// Each slot has the same six verbs, named after the table it replaced:
//
//	xAt(name)       the value, or the zero value where there is none
//	xLookup(name)   the value and whether the slot is present
//	xPut(name, v)   store, making the cell on first use
//	xDel(name)      take the slot away; a slot that is not there is a no-op
//	xLen()          how many names hold the slot (only where something asks)
//	xRange()        every name holding the slot and its value
//
// **The cells live in a slab, and a name is an index into it.** cellIndex maps
// a name to its position in cellSlab, so a read is one map lookup and a
// write to a name the shell already has is one lookup and a store into the
// slab. A cell whose last slot is taken away stays where it is, empty, because
// names come back — a function's `local` takes one away and puts it back on
// every call — and the index then has nothing to learn. Empty cells are
// compacted away once they outnumber the live ones.
//
// **The layout is what makes a subshell cheap.** A clone copies the slab —
// one allocation and a copy, where a table of names would be rebuilt key by
// key — and *shares the index* until one side adds a name, through the same
// tableShare the other copy-on-write tables use (sharedtable.go). Positions
// mean the same thing in both slabs until then, and after it each side has an
// index of its own. Measured on a `$(echo a)` loop, rebuilding a map of cells
// per clone cost 10% of the loop where this costs nothing measurable.
//
// What a subshell owns is otherwise unchanged by the cells. It gets its own
// copy of every indexed array, as it had its own Arrays; an associative
// array's *contents* are still shared until one side writes, so a write into
// an element still asks ownAssocs first. See TestACloneOwnsEveryCell and the
// store's rows of TestASubshellWriteToASharedTableStaysInTheSubshell.

// varCell is everything the store holds about one name.
type varCell struct {
	name          string
	scalar        string
	array         Array
	assoc         AssocArray
	producer      func(*Runner) string
	arrayProducer func(*Runner) []string
	assocProducer func(*Runner) AssocArray
	// assocKeys is the stored table's keys in the order a read lists them,
	// kept until the set of keys changes: see storedAssocKeys.
	assocKeys []string
	// has says which slots are present. A slot's value is its zero value
	// whenever it is absent.
	has cellSlots
	// attrs is the name's boolean attributes — see cellAttrs. A cell holding
	// attributes and no slot is still a cell: `readonly x` with x unset is a
	// name the shell knows about.
	attrs cellAttrs
}

// live reports whether the cell holds anything: a slot or an attribute.
func (c *varCell) live() bool { return c.has != 0 || c.attrs != 0 }

// cellSlots is a set of a cell's slots.
type cellSlots uint8

const (
	slotScalar cellSlots = 1 << iota
	slotArray
	slotAssoc
	slotProducer
	slotArrayProducer
	slotAssocProducer
)

// cellSlotCount is how many slots a cell has, which is the length of
// Runner.cellCount.
const cellSlotCount = 6

// slotIndex is a slot's position in Runner.cellCount.
func slotIndex(s cellSlots) int {
	switch s {
	case slotScalar:
		return 0
	case slotArray:
		return 1
	case slotAssoc:
		return 2
	case slotProducer:
		return 3
	case slotArrayProducer:
		return 4
	default:
		return 5
	}
}

// clearSlot zeroes s's value in c.
func (c *varCell) clearSlot(s cellSlots) {
	switch s {
	case slotScalar:
		c.scalar = ""
	case slotArray:
		c.array = nil
	case slotAssoc:
		c.assoc, c.assocKeys = nil, nil
	case slotProducer:
		c.producer = nil
	case slotArrayProducer:
		c.arrayProducer = nil
	default:
		c.assocProducer = nil
	}
}

// cell is name's cell, or nil. The pointer is into the slab, so it is good
// until the next cell is made.
//
// The last name found is remembered with its position, and asked first. A
// declaration asks about one name a dozen times in a row — its value, its
// arrays, its attributes, on the way in and again on the way out — and each
// was a hash of the name and a probe of the index (#6373). The position is
// sound for as long as the index says the same thing, which is until the slab
// is compacted or let go: a name is never moved otherwise, a name added goes
// on the end, and a clone's slab has the same positions as its parent's. Both
// of those places forget it.
func (r *Runner) cell(name string) *varCell {
	if r.lastCell != 0 && sameName(name, r.lastCellName) {
		return &r.cellSlab[r.lastCell-1]
	}
	r.cellProbes++
	if i, ok := r.cellIndex[name]; ok {
		r.lastCellName, r.lastCell = name, i+1
		return &r.cellSlab[i]
	}
	return nil
}

// sameName reports whether two names are one, asking the length and the first
// byte before the comparison: the names a function declares are as often as
// not one letter each, and a scan over a handful of them compares names of
// the same length that differ, each a call into the runtime (#6373).
func sameName(a, b string) bool {
	return len(a) == len(b) && (len(a) == 0 || a[0] == b[0]) && a == b
}

// cellFor is the cell a write lands in, made if the name has none.
func (r *Runner) cellFor(name string) *varCell {
	if c := r.cell(name); c != nil {
		return c
	}
	r.ownCellIndex()
	r.cellSlab = append(r.cellSlab, varCell{name: name})
	i := int32(len(r.cellSlab) - 1)
	r.cellIndex[name] = i
	r.lastCellName, r.lastCell = name, i+1
	return &r.cellSlab[i]
}

// ownCellIndex makes the index this runner's to write: a copy, where a clone
// still holds it.
func (r *Runner) ownCellIndex() {
	if mustCopyShared(&r.cellIndexShare) {
		m := maps.Clone(r.cellIndex)
		leaveShare(&r.cellIndexShare)
		r.cellIndex = m
	}
	if r.cellIndex == nil {
		r.cellIndex = map[string]int32{}
	}
}

// markSlot records that c now holds s, counting it if it did not.
func (r *Runner) markSlot(c *varCell, s cellSlots) {
	if c.has&s != 0 {
		return
	}
	if !c.live() {
		r.liveCells++
	}
	c.has |= s
	r.cellCount[slotIndex(s)]++
}

// slotCell is the cell holding s under name, or nil.
func (r *Runner) slotCell(name string, s cellSlots) *varCell {
	if c := r.cell(name); c != nil && c.has&s != 0 {
		return c
	}
	return nil
}

// dropSlot takes s away from name's cell, value and all.
func (r *Runner) dropSlot(name string, s cellSlots) {
	c := r.slotCell(name, s)
	if c == nil {
		return
	}
	c.has &^= s
	c.clearSlot(s)
	r.cellCount[slotIndex(s)]--
	if !c.live() {
		r.liveCells--
		r.compactCellsIfSparse()
	}
}

// compactCellsIfSparse rebuilds the slab without its empty cells once they
// outnumber the live ones, so that a script that makes and unsets names by
// the thousand does not carry every one of them into each subshell. Never
// while a range over the slab is running, which would see the positions
// move under it.
func (r *Runner) compactCellsIfSparse() {
	dead := len(r.cellSlab) - r.liveCells
	if dead < 256 || dead <= r.liveCells || r.cellsRanging > 0 {
		return
	}
	slab := make([]varCell, 0, r.liveCells)
	index := make(map[string]int32, r.liveCells)
	for i := range r.cellSlab {
		if c := &r.cellSlab[i]; c.live() {
			index[c.name] = int32(len(slab))
			slab = append(slab, *c)
		}
	}
	leaveShare(&r.cellIndexShare)
	r.cellSlab, r.cellIndex = slab, index
	r.lastCellName, r.lastCell = "", 0
}

// slotRange is every cell holding s, in the order the names were made. A
// write during the range is allowed; a cell made during it may or may not be
// visited, as a map's own range allows.
func (r *Runner) slotRange(s cellSlots) iter.Seq[*varCell] {
	return func(yield func(*varCell) bool) {
		if r.cellCount[slotIndex(s)] == 0 {
			return
		}
		r.cellsRanging++
		defer func() { r.cellsRanging-- }()
		for i := 0; i < len(r.cellSlab); i++ {
			if c := &r.cellSlab[i]; c.has&s != 0 && !yield(c) {
				return
			}
		}
	}
}

// vars is the stored scalar.

func (r *Runner) varsAt(name string) string {
	if c := r.cell(name); c != nil {
		return c.scalar
	}
	return ""
}

func (r *Runner) varsLookup(name string) (string, bool) {
	if c := r.cell(name); c != nil && c.has&slotScalar != 0 {
		return c.scalar, true
	}
	return "", false
}

func (r *Runner) varsPut(name, value string) {
	c := r.cellFor(name)
	r.markSlot(c, slotScalar)
	c.scalar = value
}

func (r *Runner) varsDel(name string) { r.dropSlot(name, slotScalar) }

func (r *Runner) varsLen() int { return r.cellCount[slotIndex(slotScalar)] }

func (r *Runner) varsRange() iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		for c := range r.slotRange(slotScalar) {
			if !yield(c.name, c.scalar) {
				return
			}
		}
	}
}

// arrays is the indexed array.

func (r *Runner) arraysAt(name string) Array {
	if c := r.cell(name); c != nil {
		return c.array
	}
	return nil
}

func (r *Runner) arraysLookup(name string) (Array, bool) {
	if c := r.cell(name); c != nil && c.has&slotArray != 0 {
		return c.array, true
	}
	return nil, false
}

func (r *Runner) arraysPut(name string, a Array) {
	c := r.cellFor(name)
	r.markSlot(c, slotArray)
	c.array = a
}

func (r *Runner) arraysDel(name string) { r.dropSlot(name, slotArray) }

func (r *Runner) arraysLen() int { return r.cellCount[slotIndex(slotArray)] }

func (r *Runner) arraysRange() iter.Seq2[string, Array] {
	return func(yield func(string, Array) bool) {
		for c := range r.slotRange(slotArray) {
			if !yield(c.name, c.array) {
				return
			}
		}
	}
}

// assocs is the associative array. Its contents are shared with a clone until
// one side writes: a write into one calls ownAssocs first. See sharedtable.go.

func (r *Runner) assocsAt(name string) AssocArray {
	if c := r.cell(name); c != nil {
		return c.assoc
	}
	return nil
}

func (r *Runner) assocsLookup(name string) (AssocArray, bool) {
	if c := r.cell(name); c != nil && c.has&slotAssoc != 0 {
		return c.assoc, true
	}
	return nil, false
}

func (r *Runner) assocsPut(name string, a AssocArray) {
	c := r.cellFor(name)
	r.markSlot(c, slotAssoc)
	c.assoc, c.assocKeys = a, nil
}

func (r *Runner) assocsDel(name string) { r.dropSlot(name, slotAssoc) }

func (r *Runner) assocsLen() int { return r.cellCount[slotIndex(slotAssoc)] }

func (r *Runner) assocsRange() iter.Seq2[string, AssocArray] {
	return func(yield func(string, AssocArray) bool) {
		for c := range r.slotRange(slotAssoc) {
			if !yield(c.name, c.assoc) {
				return
			}
		}
	}
}

// producers is the scalar produced when it is read: `LINENO` is wherever
// execution has reached, and `RANDOM` is a different number every time. A
// dialect fills in the ones it has through SetDynamic.

func (r *Runner) producersAt(name string) func(*Runner) string {
	if c := r.cell(name); c != nil {
		return c.producer
	}
	return nil
}

func (r *Runner) producersLookup(name string) (func(*Runner) string, bool) {
	if c := r.cell(name); c != nil && c.has&slotProducer != 0 {
		return c.producer, true
	}
	return nil, false
}

func (r *Runner) producersPut(name string, f func(*Runner) string) {
	c := r.cellFor(name)
	r.markSlot(c, slotProducer)
	c.producer = f
}

func (r *Runner) producersDel(name string) { r.dropSlot(name, slotProducer) }

func (r *Runner) producersRange() iter.Seq2[string, func(*Runner) string] {
	return func(yield func(string, func(*Runner) string) bool) {
		for c := range r.slotRange(slotProducer) {
			if !yield(c.name, c.producer) {
				return
			}
		}
	}
}

// arrayProducers is the array produced when it is read, and the call stack is
// why it exists: `BASH_SOURCE` and its relatives cannot be stored and stay
// right. A dialect fills in the names it has through SetDynamicArray.

func (r *Runner) arrayProducersAt(name string) func(*Runner) []string {
	if c := r.cell(name); c != nil {
		return c.arrayProducer
	}
	return nil
}

func (r *Runner) arrayProducersLookup(name string) (func(*Runner) []string, bool) {
	if c := r.cell(name); c != nil && c.has&slotArrayProducer != 0 {
		return c.arrayProducer, true
	}
	return nil, false
}

func (r *Runner) arrayProducersPut(name string, f func(*Runner) []string) {
	c := r.cellFor(name)
	r.markSlot(c, slotArrayProducer)
	c.arrayProducer = f
}

func (r *Runner) arrayProducersDel(name string) { r.dropSlot(name, slotArrayProducer) }

func (r *Runner) arrayProducersRange() iter.Seq2[string, func(*Runner) []string] {
	return func(yield func(string, func(*Runner) []string) bool) {
		for c := range r.slotRange(slotArrayProducer) {
			if !yield(c.name, c.arrayProducer) {
				return
			}
		}
	}
}

// assocProducers is the keyed table produced when it is read: a parameter that
// answers "which functions exist, and what is each one's body" is a
// name-to-value map read at the moment it is asked. A dialect fills in the
// names it has through SetDynamicAssoc.

func (r *Runner) assocProducersAt(name string) func(*Runner) AssocArray {
	if c := r.cell(name); c != nil {
		return c.assocProducer
	}
	return nil
}

func (r *Runner) assocProducersLookup(name string) (func(*Runner) AssocArray, bool) {
	if c := r.cell(name); c != nil && c.has&slotAssocProducer != 0 {
		return c.assocProducer, true
	}
	return nil, false
}

func (r *Runner) assocProducersPut(name string, f func(*Runner) AssocArray) {
	c := r.cellFor(name)
	r.markSlot(c, slotAssocProducer)
	c.assocProducer = f
}

func (r *Runner) assocProducersDel(name string) { r.dropSlot(name, slotAssocProducer) }

func (r *Runner) assocProducersRange() iter.Seq2[string, func(*Runner) AssocArray] {
	return func(yield func(string, func(*Runner) AssocArray) bool) {
		for c := range r.slotRange(slotAssocProducer) {
			if !yield(c.name, c.assocProducer) {
				return
			}
		}
	}
}

// ownVarTables is clone's half for the store: the clone starts from `c :=
// *r`, which shares the parent's slab and index. It gets a slab of its own —
// a copy, with every indexed array in it copied too, so the subshell does not
// write the parent's array through its own cell — and shares the index until
// either side adds a name. An associative array's contents are shared until
// one side writes: see shareTables and ownAssocs.
func (c *Runner) ownVarTables(r *Runner) {
	c.cellSlab = slices.Clone(r.cellSlab)
	for i := range c.cellSlab {
		if a := c.cellSlab[i].array; a != nil {
			c.cellSlab[i].array = a.clone()
		}
	}
	c.cellIndexShare = shareTable(&r.cellIndexShare)
	c.cellsRanging = 0
}

// releaseVarTables gives up a finished clone's hold on the store's index and
// lets go of the store, so that the parent adds its next name in place and a
// write the clone made by mistake afterwards lands in a store of its own.
func (r *Runner) releaseVarTables() {
	leaveShare(&r.cellIndexShare)
	leaveShare(&r.assocShare)
	r.cellIndex, r.cellSlab = nil, nil
	r.lastCellName, r.lastCell = "", 0
	r.cellCount, r.liveCells = [cellSlotCount]int{}, 0
	r.attrCount, r.attrTotal = [cellAttrCount]int{}, 0
}

// ownAssocs makes the associative arrays this runner's to write: every table
// under a name, since a write to an element is a write into the inner map.
func (r *Runner) ownAssocs() {
	if mustCopyShared(&r.assocShare) {
		for c := range r.slotRange(slotAssoc) {
			c.assoc = c.assoc.clone()
		}
		leaveShare(&r.assocShare)
	}
}

// varTables is a copy of the three stored slots — scalar, indexed and
// associative — for a caller that has to put them back or ask whether anything
// wrote them. The producers are not in it: see saveExpansionTables.
type varTables struct {
	vars   map[string]string
	arrays map[string]Array
	assoc  map[string]AssocArray
}

// saveVarTables takes the copy, one level deeper for the two array slots,
// whose values are containers.
func (r *Runner) saveVarTables() varTables {
	t := varTables{
		vars:   make(map[string]string, r.varsLen()),
		arrays: make(map[string]Array, r.arraysLen()),
		assoc:  make(map[string]AssocArray, r.assocsLen()),
	}
	for i := range r.cellSlab {
		c := &r.cellSlab[i]
		if c.has&slotScalar != 0 {
			t.vars[c.name] = c.scalar
		}
		if c.has&slotArray != 0 {
			t.arrays[c.name] = c.array.clone()
		}
		if c.has&slotAssoc != 0 {
			t.assoc[c.name] = c.assoc.clone()
		}
	}
	return t
}

// restoreVarTables puts the copy back, leaving the producers as they are. The
// tables put back are the copy's own, so nothing the parent holds is shared.
func (r *Runner) restoreVarTables(t varTables) {
	leaveShare(&r.assocShare)
	for _, s := range []cellSlots{slotScalar, slotArray, slotAssoc} {
		for c := range r.slotRange(s) {
			c.has &^= s
			c.clearSlot(s)
			if !c.live() {
				r.liveCells--
			}
		}
		r.cellCount[slotIndex(s)] = 0
	}
	for name, v := range t.vars {
		r.varsPut(name, v)
	}
	for name, a := range t.arrays {
		r.arraysPut(name, a)
	}
	for name, a := range t.assoc {
		r.assocsPut(name, a)
	}
}

// varTablesWroteSince reports whether any of the three stored slots has
// changed since the copy was taken.
func (r *Runner) varTablesWroteSince(t varTables) bool {
	if r.varsLen() != len(t.vars) || r.arraysLen() != len(t.arrays) || r.assocsLen() != len(t.assoc) {
		return true
	}
	for i := range r.cellSlab {
		c := &r.cellSlab[i]
		if c.has&slotScalar != 0 {
			if v, ok := t.vars[c.name]; !ok || v != c.scalar {
				return true
			}
		}
		if c.has&slotArray != 0 {
			if a, ok := t.arrays[c.name]; !ok || !c.array.equal(a) {
				return true
			}
		}
		if c.has&slotAssoc != 0 {
			if a, ok := t.assoc[c.name]; !ok || !c.assoc.equal(a) {
				return true
			}
		}
	}
	return false
}

// The store as an embedder reaches it. Each of these is the raw operation on
// the store, with none of an assignment's semantics — no attribute, no export,
// no producer consulted.

// SeedVars stores each scalar as given and returns r, so that a Runner literal
// can be seeded where it is written: `(&interp.Runner{…}).SeedVars(m)` is what
// a literal's Vars field was before the store was unexported. The map is
// copied, not kept.
func (r *Runner) SeedVars(vars map[string]string) *Runner {
	for name, value := range vars {
		r.varsPut(name, value)
	}
	return r
}

// StoredScalar is the scalar the store holds under name, without a read's
// fold, producer or reference: what Vars[name] was.
func (r *Runner) StoredScalar(name string) (string, bool) { return r.varsLookup(name) }

// StoredScalars is every name holding a stored scalar and its value, in no
// particular order. A produced parameter, an array and an associative array
// are not in it; an indexed array may keep a copy of one element here.
func (r *Runner) StoredScalars() iter.Seq2[string, string] { return r.varsRange() }

// StoredArrayNames is every name holding an indexed array, in no particular
// order.
func (r *Runner) StoredArrayNames() iter.Seq[string] {
	return func(yield func(string) bool) {
		for name := range r.arraysRange() {
			if !yield(name) {
				return
			}
		}
	}
}

// DropStoredScalar takes the stored scalar away from name and nothing else:
// not an unset, which would also mark the name and its attributes.
func (r *Runner) DropStoredScalar(name string) { r.varsDel(name) }

// DropStoredArray takes the indexed array away from name and nothing else.
func (r *Runner) DropStoredArray(name string) { r.arraysDel(name) }

// CopyVariablesFrom gives r a copy of src's scalars and indexed arrays, so
// that what r assigns stays in r.
func (r *Runner) CopyVariablesFrom(src *Runner) {
	for name, value := range src.varsRange() {
		r.varsPut(name, value)
	}
	for name, a := range src.arraysRange() {
		r.arraysPut(name, a.clone())
	}
}

// StoreScalar stores value as name's scalar and nothing else: what an
// assignment to Vars[name] was.
func (r *Runner) StoreScalar(name, value string) { r.varsPut(name, value) }

// StoreArray stores a as name's indexed array and nothing else: what an
// assignment to Arrays[name] was. The array is kept, not copied.
func (r *Runner) StoreArray(name string, a Array) { r.arraysPut(name, a) }

// StoreAssoc stores a as name's associative array and nothing else: what an
// assignment to AssocArrays[name] was. The table is kept, not copied.
func (r *Runner) StoreAssoc(name string, a AssocArray) {
	r.ownAssocs()
	r.assocsPut(name, a)
}

// AssocProducer is the whole-table producer registered for name with
// SetDynamicAssoc, or nil.
func (r *Runner) AssocProducer(name string) func(*Runner) AssocArray {
	return r.assocProducersAt(name)
}

// storedAssocKeys is a's keys in sorted order, kept with the cell when a is
// the table stored under name, so that listing an unchanged table is a copy
// rather than a sort (#6384). Sorted is every dialect's order here — measured
// 2026-10-11, ksh93u+ lists a table's keys in byte order and bash 5.3 and zsh
// 5.9.2 each in an order of their own hash table's making, which is not ours to
// imitate (#3304) — so the cache changes when the order is computed, never
// what it is.
//
// The keys are dropped whenever the set of them can have changed: a table
// stored or taken away (assocsPut, clearSlot) and a key added or removed in
// place (assocKeySetChanged, at the three writers that do that). A count that
// no longer matches is the backstop for a write that reached the table any
// other way. The caller gets a copy, so nothing it does to the list reaches
// the next reader.
func (r *Runner) storedAssocKeys(name string, a AssocArray) []string {
	c := r.cell(name)
	if c == nil || c.has&slotAssoc == 0 || len(a) == 0 || !sameAssoc(c.assoc, a) {
		return a.keys()
	}
	if c.assocKeys == nil || len(c.assocKeys) != len(a) {
		c.assocKeys = a.keys()
	}
	return slices.Clone(c.assocKeys)
}

// assocKeySorts counts the key lists AssocArray.keys has built, so a test can
// say how many sorts a sequence of reads cost.
var assocKeySorts atomic.Int64

// assocKeySetChanged drops the listing order kept for name's stored table,
// after a key was added to it or removed from it in place.
func (r *Runner) assocKeySetChanged(name string) {
	if c := r.cell(name); c != nil {
		c.assocKeys = nil
	}
}

// sameAssoc reports whether two tables are one map rather than two equal ones.
func sameAssoc(a, b AssocArray) bool {
	return reflect.ValueOf(a).UnsafePointer() == reflect.ValueOf(b).UnsafePointer()
}

// storedScalarShortcut switches storedScalarOnly's answer on, so a test can run
// a script with the whole chain answering every read and require the same
// output — see TestAPlainParameterWordAnswersAsTheFullRoadDoes.
var storedScalarShortcut = true

// slotsOf is which slots name's cell holds, none where it has no cell. A
// caller asking about several kinds of one name asks once.
func (r *Runner) slotsOf(name string) cellSlots {
	if c := r.cell(name); c != nil {
		return c.has
	}
	return 0
}

// cellHolds reports whether name's cell holds any of the slots in s.
func (r *Runner) cellHolds(name string, s cellSlots) bool {
	c := r.cell(name)
	return c != nil && c.has&s != 0
}

// storedScalarOnly is name's stored scalar where that is the whole of what a
// read of the name answers before its attribute fold: the cell holds a stored
// scalar and nothing else, and nothing Runner.storedValue asks ahead of the
// store can answer instead — no namespace block or member path, no reference,
// no selected frame, and not the produced pipeline status. Every check is a
// comparison or a lookup in a table that is empty in nearly every shell, so an
// ordinary variable is read with one lookup in the index (#6353).
//
// The chain it short-cuts asks, in order: the namespace and compound-member
// renames (a dot in the name, or a namespace block running), the reference
// walk, the pipeline status, a producer, a compound (which has no stored
// scalar), the selected frame, the two array slots, the two table slots and
// then the scalar. Each of those is excluded here by the cell's slots or by
// the guard beside it, so the two answers cannot differ.
func (r *Runner) storedScalarOnly(name string) (string, bool) {
	c := r.cell(name)
	if c == nil || c.has != slotScalar {
		return "", false
	}
	if r.namespace != "" || r.frameSelected || name == r.pipeStatusName || strings.IndexByte(name, '.') >= 0 {
		return "", false
	}
	if _, ok := r.nameref[name]; ok {
		return "", false
	}
	return c.scalar, true
}

// cellAttrs is a set of the boolean attributes a name carries, kept in its
// cell beside its value (#6373).
//
// Each was a table of its own on Runner, keyed by name, until then. A
// declaration inside a function displaces every attribute of the name it
// shadows and the return puts every one back, so one `local a` was a lookup
// and a delete in each table on the way in and a write or a delete in each on
// the way out — about forty map operations a name, which was the largest
// single share of what `local` cost. In the cell the whole set is one word: the
// shadow reads it with the name's one lookup and the return writes it back the
// same way. A subshell copies it with the slab, where each table was a clone
// of its own.
//
// What each attribute means is unchanged and documented here, where it lives:
//
//	attrInteger      the name evaluates what is assigned to it: with the
//	                 attribute, `n=5+2` stores 7 rather than the four
//	                 characters. A property of the name and not of the
//	                 assignment, which is why it is recorded per name.
//	attrFloatExponent
//	                 the float attribute came from the `E` letter rather
//	                 than from `F` — a format and not a width. See
//	                 interp/floatformat.go.
//	attrLower, attrUpper
//	                 the case attributes — `declare -l` and `-u` — which fold
//	                 what is assigned to the name, the same shape the integer
//	                 attribute has.
//	attrUnique       only the first occurrence of each element is kept —
//	                 `typeset -U`. Consulted by every write rather than by the
//	                 declaration that set it, which is what makes
//	                 `path=( new "${path[@]}" )` drop the copy of `new` that
//	                 was already there. See Semantics.DeclareOptions for who
//	                 may set it.
//	attrHidden       the value is kept out of every listing that would write
//	                 one — `typeset -H`. It changes nothing about a read. See
//	                 Semantics.DeclareOptions for who may set it.
//	attrTraced       the trace attribute — `typeset -t`. Read by the DEBUG
//	                 trap and by nothing else; it changes no value.
//	attrDeclaredBare a declaration brought the name into being with no
//	                 letters, no value and nothing in it — declared and unset
//	                 at once. See baredeclaration.go.
//	attrDeclaredHoldingNothing
//	                 a declaration brought the name into being holding
//	                 nothing, under the reading where a valueless declaration
//	                 leaves the name unset — with letters or without. See
//	                 Runner.ParameterAttributes (#5439).
//	attrUnsetLeftItDeclared
//	                 an `unset` emptied the binding while the scope that
//	                 declared it was running — the state a bare declaration
//	                 leaves, reached the other way. See
//	                 interp/unsetenclosinglocal.go.
//	attrReadonly     the name refuses assignment.
//	attrHideInScope  the hide-in-scope attribute — `typeset -h`, and `+h` to
//	                 take it off. It decides only where a scope has displaced
//	                 the name. See hideinscope.go.
//	attrDeclaredOnlyCompound
//	                 the name's array or table came from a declaration's
//	                 letters alone, with nothing yet written to it. See
//	                 compounddeclaredonly.go.
//	attrHeldAnElement
//	                 the name's indexed array has had an element in it at some
//	                 point and may have none now — a third state beside the one
//	                 above, since `typeset -a b=()` ends the declared-only state
//	                 without putting an element in. See
//	                 Runner.bareAssignmentValue.
//	attrScalarUnderTheArrayLetter
//	                 the name's indexed array held no element, and never had,
//	                 when a whole-name scalar store landed on it; one listing
//	                 writes it as that scalar until an array write touches it.
//	                 See Semantics.ScalarHeldUnderTheArrayLetterListsAsAScalar.
//	attrRemoved      `unset` took the name away. It came from the environment
//	                 rather than from the store, so deleting the stored value
//	                 cannot hide it: the environment is a second source and a
//	                 read consults both. Nothing clears the record and nothing
//	                 needs to — a later assignment puts the name in the store,
//	                 which every read asks first.
//	attrDeclaredEmpty
//	                 a declaration gave the name its value, in the dialect that
//	                 reads a name declared without one as set: the shell reads
//	                 it as empty and no child is told about it. Cleared by an
//	                 assignment, and by a scope that shadowed the name going
//	                 away.
//	attrExportSpoken, attrExportOn
//	                 the export attribute, which has three states rather than
//	                 two: spoken and on goes into a command's environment,
//	                 spoken and off was taken out of it (`export -n`, `declare
//	                 +x`), and unspoken is a name nothing has said either about.
//	                 See exportedLookup.
type cellAttrs uint32

const (
	attrInteger cellAttrs = 1 << iota
	attrFloatExponent
	attrLower
	attrUpper
	attrUnique
	attrHidden
	attrTraced
	attrDeclaredBare
	attrDeclaredHoldingNothing
	attrUnsetLeftItDeclared
	attrReadonly
	attrHideInScope
	attrDeclaredOnlyCompound
	attrHeldAnElement
	attrScalarUnderTheArrayLetter
	attrRemoved
	attrDeclaredEmpty
	attrExportSpoken
	attrExportOn
)

// cellAttrCount is how many attributes a cell has, which is the length of
// Runner.attrCount.
const cellAttrCount = 19

// attrIndex is an attribute's position in Runner.attrCount.
func attrIndex(a cellAttrs) int { return bits.TrailingZeros32(uint32(a)) }

// attrHas reports whether name carries a.
//
// An attribute no name carries is answered without the lookup, which is what
// an empty table used to answer for free: most of these are carried by no
// name at all in most shells, and every assignment asks about several.
func (r *Runner) attrHas(name string, a cellAttrs) bool {
	if r.attrCount[attrIndex(a)] == 0 {
		return false
	}
	c := r.cell(name)
	return c != nil && c.attrs&a != 0
}

// attrsOf is every attribute name carries, for a caller asking about several
// of one name.
func (r *Runner) attrsOf(name string) cellAttrs {
	if r.attrTotal == 0 {
		return 0
	}
	if c := r.cell(name); c != nil {
		return c.attrs
	}
	return 0
}

// attrSet gives name a, making the cell if the name has none.
func (r *Runner) attrSet(name string, a cellAttrs) {
	if c := r.cell(name); c != nil && c.attrs&a == a {
		return
	}
	r.setAttrs(name, a, a)
}

// attrClear takes a off name; a name without it is a no-op.
func (r *Runner) attrClear(name string, a cellAttrs) {
	if r.attrCount[attrIndex(a)] == 0 {
		return
	}
	if c := r.cell(name); c != nil && c.attrs&a != 0 {
		r.setAttrs(name, a, 0)
	}
}

// attrPut gives name a or takes it off.
func (r *Runner) attrPut(name string, a cellAttrs, on bool) {
	if on {
		r.attrSet(name, a)
	} else {
		r.attrClear(name, a)
	}
}

// setAttrs makes the attributes in mask exactly those of to that are in it,
// leaving the rest of name's alone — the one write a shadow's return needs to
// put a whole set back. A cell is made only where something is being set.
func (r *Runner) setAttrs(name string, mask, to cellAttrs) {
	to &= mask
	if to == 0 && r.attrTotal == 0 {
		return
	}
	c := r.cell(name)
	if c == nil {
		if to == 0 {
			return
		}
		c = r.cellFor(name)
	}
	was := c.live()
	changed := (c.attrs ^ to) & mask
	if changed == 0 {
		return
	}
	for m := changed; m != 0; m &= m - 1 {
		bit := m & -m
		if to&bit != 0 {
			r.attrCount[attrIndex(bit)]++
			r.attrTotal++
		} else {
			r.attrCount[attrIndex(bit)]--
			r.attrTotal--
		}
	}
	c.attrs = c.attrs&^mask | to
	switch now := c.live(); {
	case now && !was:
		r.liveCells++
	case !now && was:
		r.liveCells--
		r.compactCellsIfSparse()
	}
}

// attrLen is how many names carry a.
func (r *Runner) attrLen(a cellAttrs) int { return r.attrCount[attrIndex(a)] }

// attrNames is every name carrying a, in the order the names were made. A
// write during the range is allowed, as slotRange allows one.
func (r *Runner) attrNames(a cellAttrs) iter.Seq[string] {
	return func(yield func(string) bool) {
		if r.attrCount[attrIndex(a)] == 0 {
			return
		}
		r.cellsRanging++
		defer func() { r.cellsRanging-- }()
		for i := 0; i < len(r.cellSlab); i++ {
			if c := &r.cellSlab[i]; c.attrs&a != 0 && !yield(c.name) {
				return
			}
		}
	}
}

// exported is the export attribute: see attrExportSpoken. The verbs are the
// ones the map they replaced had, so that `on, spoken := r.exportedLookup(n)`
// reads as `on, spoken := r.exported[n]` did.

// exportedAt reports whether name goes into a command's environment.
func (r *Runner) exportedAt(name string) bool { return r.attrHas(name, attrExportOn) }

// exportedLookup is the attribute and whether anything has spoken about it.
func (r *Runner) exportedLookup(name string) (on, spoken bool) {
	at := r.attrsOf(name)
	return at&attrExportOn != 0, at&attrExportSpoken != 0
}

// exportedPut records name as exported or as taken out of the environment.
func (r *Runner) exportedPut(name string, on bool) {
	if !on {
		// Out of the environment, so there is no copy left to hold; an
		// `export` after it hands a child the value the shell has.
		r.releaseEnvCopy(name)
	}
	to := attrExportSpoken
	if on {
		to |= attrExportOn
	}
	r.setAttrs(name, attrExportSpoken|attrExportOn, to)
}

// exportedDel forgets anything said about name's export.
func (r *Runner) exportedDel(name string) {
	r.releaseEnvCopy(name)
	r.setAttrs(name, attrExportSpoken|attrExportOn, 0)
}

// exportedRange is every name something has said the export of, and whether
// it is exported.
func (r *Runner) exportedRange() iter.Seq2[string, bool] {
	return func(yield func(string, bool) bool) {
		if r.attrCount[attrIndex(attrExportSpoken)] == 0 {
			return
		}
		r.cellsRanging++
		defer func() { r.cellsRanging-- }()
		for i := 0; i < len(r.cellSlab); i++ {
			c := &r.cellSlab[i]
			if c.attrs&attrExportSpoken != 0 && !yield(c.name, c.attrs&attrExportOn != 0) {
				return
			}
		}
	}
}

// saveAttrs is a copy of the attributes in mask, for a caller that has to put
// them back or ask whether anything wrote them — see expansionTables.
func (r *Runner) saveAttrs(mask cellAttrs) map[string]cellAttrs {
	m := map[string]cellAttrs{}
	if r.attrTotal == 0 {
		return m
	}
	for i := range r.cellSlab {
		if c := &r.cellSlab[i]; c.attrs&mask != 0 {
			m[c.name] = c.attrs & mask
		}
	}
	return m
}

// restoreAttrs puts back the attributes in mask as saveAttrs copied them,
// taking them off every name the copy does not hold.
func (r *Runner) restoreAttrs(mask cellAttrs, saved map[string]cellAttrs) {
	r.cellsRanging++
	for i := 0; i < len(r.cellSlab); i++ {
		if c := &r.cellSlab[i]; c.attrs&mask != 0 {
			if _, ok := saved[c.name]; !ok {
				r.setAttrs(c.name, mask, 0)
			}
		}
	}
	r.cellsRanging--
	for name, at := range saved {
		r.setAttrs(name, mask, at)
	}
}

// attrsWroteSince reports whether the attributes in mask differ from what
// saveAttrs copied.
func (r *Runner) attrsWroteSince(mask cellAttrs, saved map[string]cellAttrs) bool {
	n := 0
	for i := range r.cellSlab {
		c := &r.cellSlab[i]
		if c.attrs&mask == 0 {
			continue
		}
		n++
		if saved[c.name] != c.attrs&mask {
			return true
		}
	}
	return n != len(saved)
}

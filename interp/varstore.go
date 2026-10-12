// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"iter"
	"maps"
)

// The variable store's own surface: every read and write of the six tables a
// name's value can live in goes through a method in this file.
//
// The tables are Vars, Arrays, AssocArrays, Dynamic, DynamicArrays and
// DynamicAssocs. A name can be in several of them at once — an array keeps a
// copy of one element in Vars, a produced parameter keeps what was assigned to
// it beside the producer — so the read path in Runner.storedValue asks about a
// dozen of them, in a fixed order, for one scalar. Answering that in one
// lookup needs one record per name that owns its invariants, and a record
// cannot own anything while ~400 sites write the maps directly (#6353). This
// file is the first step toward that record: the representation is unchanged
// and every method is the map operation it replaces, so routing a site
// through it changes nothing a script can see. What it buys is that the next
// step changes this file rather than every caller.
//
// Each table has the same six verbs, named after the table:
//
//	xAt(name)       the value, or the zero value where there is none
//	xLookup(name)   the value and whether the name is there
//	xPut(name, v)   store, making the table on first use
//	xDel(name)      take the name out; a name that is not there is no-op
//	xLen()          how many names
//	xRange()        every name and value, in the map's own order
//
// A write that may meet a table shared with a clone still asks for it first —
// ownAssocs for the associative arrays — exactly as it did before the
// methods: this file routes the write, it does not decide whether it is
// allowed. See sharedtable.go.

// vars is Vars, the scalar table.

func (r *Runner) varsAt(name string) string { return r.Vars[name] }

func (r *Runner) varsLookup(name string) (string, bool) {
	v, ok := r.Vars[name]
	return v, ok
}

func (r *Runner) varsPut(name, value string) {
	if r.Vars == nil {
		r.Vars = map[string]string{}
	}
	r.Vars[name] = value
}

func (r *Runner) varsDel(name string) { delete(r.Vars, name) }

func (r *Runner) varsLen() int { return len(r.Vars) }

func (r *Runner) varsRange() iter.Seq2[string, string] { return maps.All(r.Vars) }

// arrays is Arrays, the indexed arrays.

func (r *Runner) arraysAt(name string) Array { return r.Arrays[name] }

func (r *Runner) arraysLookup(name string) (Array, bool) {
	a, ok := r.Arrays[name]
	return a, ok
}

func (r *Runner) arraysPut(name string, a Array) {
	if r.Arrays == nil {
		r.Arrays = map[string]Array{}
	}
	r.Arrays[name] = a
}

func (r *Runner) arraysDel(name string) { delete(r.Arrays, name) }

func (r *Runner) arraysLen() int { return len(r.Arrays) }

func (r *Runner) arraysRange() iter.Seq2[string, Array] { return maps.All(r.Arrays) }

// assocs is AssocArrays, the associative arrays. The table is shared with a
// clone until one side writes: a write calls ownAssocs first. See
// sharedtable.go.

func (r *Runner) assocsAt(name string) AssocArray { return r.AssocArrays[name] }

func (r *Runner) assocsLookup(name string) (AssocArray, bool) {
	a, ok := r.AssocArrays[name]
	return a, ok
}

func (r *Runner) assocsPut(name string, a AssocArray) {
	if r.AssocArrays == nil {
		r.AssocArrays = map[string]AssocArray{}
	}
	r.AssocArrays[name] = a
}

func (r *Runner) assocsDel(name string) { delete(r.AssocArrays, name) }

func (r *Runner) assocsLen() int { return len(r.AssocArrays) }

func (r *Runner) assocsRange() iter.Seq2[string, AssocArray] { return maps.All(r.AssocArrays) }

// producers is Dynamic, the scalars produced when they are read.

func (r *Runner) producersAt(name string) func(*Runner) string { return r.Dynamic[name] }

func (r *Runner) producersLookup(name string) (func(*Runner) string, bool) {
	f, ok := r.Dynamic[name]
	return f, ok
}

func (r *Runner) producersPut(name string, f func(*Runner) string) {
	if r.Dynamic == nil {
		r.Dynamic = map[string]func(*Runner) string{}
	}
	r.Dynamic[name] = f
}

func (r *Runner) producersDel(name string) { delete(r.Dynamic, name) }

func (r *Runner) producersLen() int { return len(r.Dynamic) }

func (r *Runner) producersRange() iter.Seq2[string, func(*Runner) string] {
	return maps.All(r.Dynamic)
}

// arrayProducers is DynamicArrays.

func (r *Runner) arrayProducersAt(name string) func(*Runner) []string {
	return r.DynamicArrays[name]
}

func (r *Runner) arrayProducersLookup(name string) (func(*Runner) []string, bool) {
	f, ok := r.DynamicArrays[name]
	return f, ok
}

func (r *Runner) arrayProducersPut(name string, f func(*Runner) []string) {
	if r.DynamicArrays == nil {
		r.DynamicArrays = map[string]func(*Runner) []string{}
	}
	r.DynamicArrays[name] = f
}

func (r *Runner) arrayProducersDel(name string) { delete(r.DynamicArrays, name) }

func (r *Runner) arrayProducersLen() int { return len(r.DynamicArrays) }

func (r *Runner) arrayProducersRange() iter.Seq2[string, func(*Runner) []string] {
	return maps.All(r.DynamicArrays)
}

// assocProducers is DynamicAssocs.

func (r *Runner) assocProducersAt(name string) func(*Runner) AssocArray {
	return r.DynamicAssocs[name]
}

func (r *Runner) assocProducersLookup(name string) (func(*Runner) AssocArray, bool) {
	f, ok := r.DynamicAssocs[name]
	return f, ok
}

func (r *Runner) assocProducersPut(name string, f func(*Runner) AssocArray) {
	if r.DynamicAssocs == nil {
		r.DynamicAssocs = map[string]func(*Runner) AssocArray{}
	}
	r.DynamicAssocs[name] = f
}

func (r *Runner) assocProducersDel(name string) { delete(r.DynamicAssocs, name) }

func (r *Runner) assocProducersLen() int { return len(r.DynamicAssocs) }

func (r *Runner) assocProducersRange() iter.Seq2[string, func(*Runner) AssocArray] {
	return maps.All(r.DynamicAssocs)
}

// ownVarTables is clone's half for the variable tables: the clone starts from
// `c := *r`, which shares every map, and these are made its own here. Vars is
// made rather than cloned because a nil one used to be unsafe to write; the
// arrays copy their elements too, so the subshell does not write the parent's
// array through its own name table. The associative arrays are shared until
// one side writes — see shareTables.
func (c *Runner) ownVarTables(r *Runner) {
	c.Vars = make(map[string]string, len(r.Vars))
	for k, v := range r.Vars {
		c.Vars[k] = v
	}
	c.Arrays = make(map[string]Array, len(r.Arrays))
	for k, v := range r.Arrays {
		c.Arrays[k] = v.clone()
	}
}

// ownProducerTables makes the three producer tables the clone's own. Dynamic
// is built lazily — `_` and `LINENO` are installed into it on the way past —
// so the shell itself writes it, on whatever goroutine a substitution runs.
func (c *Runner) ownProducerTables(r *Runner) {
	c.Dynamic = maps.Clone(r.Dynamic)
	c.DynamicArrays = maps.Clone(r.DynamicArrays)
	c.DynamicAssocs = maps.Clone(r.DynamicAssocs)
}

// ownAssocs makes the associative arrays this runner's to write: the table of
// names and every table under a name, since a write to an element is a write
// into the inner map.
func (r *Runner) ownAssocs() {
	if mustCopyShared(&r.assocShare) {
		m := cloneAssocTables(r.AssocArrays)
		leaveShare(&r.assocShare)
		r.AssocArrays = m
	}
}

// replaceAssocs puts tables this runner made in place of the ones it held.
func (r *Runner) replaceAssocs(m map[string]AssocArray) {
	leaveShare(&r.assocShare)
	r.AssocArrays = m
}

// varTables is a copy of the three stored tables, for a caller that has to
// put them back or ask whether anything wrote them. See saveExpansionTables.
type varTables struct {
	vars   map[string]string
	arrays map[string]Array
	assoc  map[string]AssocArray
}

// saveVarTables takes the copy, one level deeper for the two array tables,
// whose values are containers.
func (r *Runner) saveVarTables() varTables {
	return varTables{
		vars:   maps.Clone(r.Vars),
		arrays: cloneArrays(r.Arrays),
		assoc:  cloneAssoc(r.AssocArrays),
	}
}

// restoreVarTables puts the copy back.
func (r *Runner) restoreVarTables(t varTables) {
	r.Vars, r.Arrays = t.vars, t.arrays
	r.replaceAssocs(t.assoc)
}

// varTablesWroteSince reports whether any of the three has changed since the
// copy was taken.
func (r *Runner) varTablesWroteSince(t varTables) bool {
	return !maps.Equal(t.vars, r.Vars) ||
		!arraysEqual(t.arrays, r.Arrays) ||
		!assocEqual(t.assoc, r.AssocArrays)
}

// The store as an embedder reaches it. Each of these is the raw table
// operation, with none of an assignment's semantics — no attribute, no
// export, no producer consulted — which is what a caller writing into the
// tables directly used to get.

// SeedVars stores each scalar as given, which is what a Vars field in a
// Runner literal does. The map is copied, not kept.
func (r *Runner) SeedVars(vars map[string]string) {
	for name, value := range vars {
		r.varsPut(name, value)
	}
}

// StoredScalars is every name the scalar table holds and its stored value, in
// no particular order. A produced parameter, an array and an associative
// array are not in it; an indexed array may keep a copy of one element here.
func (r *Runner) StoredScalars() iter.Seq2[string, string] { return r.varsRange() }

// StoredArrayNames is every name the indexed-array table holds, in no
// particular order.
func (r *Runner) StoredArrayNames() iter.Seq[string] {
	return func(yield func(string) bool) {
		for name := range r.arraysRange() {
			if !yield(name) {
				return
			}
		}
	}
}

// DropStoredArray takes a name out of the indexed-array table and nothing
// else: not an unset, which would also mark the name and its attributes.
func (r *Runner) DropStoredArray(name string) { r.arraysDel(name) }

// CopyVariablesFrom gives r a copy of src's scalars and indexed arrays, so that
// what r assigns stays in r.
func (r *Runner) CopyVariablesFrom(src *Runner) {
	for name, value := range src.varsRange() {
		r.varsPut(name, value)
	}
	for name, a := range src.arraysRange() {
		r.arraysPut(name, a.clone())
	}
}

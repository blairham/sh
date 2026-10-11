// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// scopeSpares holds the record tables of scopes that have closed, emptied,
// for the next scope or declaration to write into.
//
// A scope keeps what a declaration displaced in a table per kind of record —
// the value, whether it existed, the frozen attribute, the export record and
// a dozen more — and each table is a Go map, which is two allocations before
// it holds anything. So `local a` made about thirty allocations for one name,
// and every function call two more for the value tables pushScope opens
// eagerly, against an empty call that made none of its own (#6361). The
// unwind already deletes every record it puts back, so the tables are empty
// when the scope closes; keeping them for the next call makes a declaration
// cost the writes it does and not the tables it writes into. This is the
// variable-store sibling of what #6100 did for the option table.
//
// What a scope sees is unchanged, including a table's nil-ness: a scope still
// starts with every lazily-made table nil, and is handed a spare only at the
// moment it would have made one.
//
// Per runner, and never shared: a clone starts with none (see ownTables), so a
// subshell running beside its parent cannot be handed a table the parent is
// writing. A popped scope is released only by the runner popping it, after
// everything that reads its tables has run.
type scopeSpares struct {
	bools  []map[string]bool
	strs   []map[string]string
	arrays []map[string]Array
	assocs []map[string]AssocArray
	attrs  []map[string]nameAttributes
}

// maxScopeSpares bounds each list. A call stack deeper than this holds more
// tables at once than are kept, and the surplus is left to the collector, so
// one deep recursion does not pin its tables for the life of the shell.
const maxScopeSpares = 256

func takeSpare[M ~map[string]V, V any](list *[]M) M {
	if n := len(*list); n > 0 {
		m := (*list)[n-1]
		(*list)[n-1] = nil
		*list = (*list)[:n-1]
		return m
	}
	return M{}
}

// giveSpare empties the table and keeps it, unless the list is full. The
// field it came from is cleared so nothing reads the table through the
// closed scope.
func giveSpare[M ~map[string]V, V any](list *[]M, field *M) {
	m := *field
	*field = nil
	if m == nil || len(*list) >= maxScopeSpares {
		return
	}
	clear(m)
	*list = append(*list, m)
}

func (r *Runner) spareBools() map[string]bool { return takeSpare(&r.spares.bools) }

func (r *Runner) spareStrings() map[string]string { return takeSpare(&r.spares.strs) }

func (r *Runner) spareArrays() map[string]Array { return takeSpare(&r.spares.arrays) }

func (r *Runner) spareAssocs() map[string]AssocArray { return takeSpare(&r.spares.assocs) }

func (r *Runner) spareAttrs() map[string]nameAttributes { return takeSpare(&r.spares.attrs) }

// releaseScopeTables hands the closed scope's record tables back for reuse.
// Only the tables a declaration fills on every name are kept; the rest are
// rare enough that a table each costs nothing worth saving.
func (r *Runner) releaseScopeTables(sc *scope) {
	s := &r.spares
	giveSpare(&s.strs, &sc.saved)
	giveSpare(&s.bools, &sc.existed)
	giveSpare(&s.bools, &sc.removedBefore)
	giveSpare(&s.bools, &sc.savedReadonly)
	giveSpare(&s.bools, &sc.savedHideInScope)
	giveSpare(&s.bools, &sc.hiddenShadow)
	giveSpare(&s.attrs, &sc.savedAttrs)
	giveSpare(&s.bools, &sc.exportedSpoken)
	giveSpare(&s.bools, &sc.savedExported)
	giveSpare(&s.strs, &sc.savedAssigned)
	giveSpare(&s.bools, &sc.assignedSpoken)
	giveSpare(&s.arrays, &sc.savedArrays)
	giveSpare(&s.bools, &sc.arrayExisted)
	giveSpare(&s.assocs, &sc.savedAssoc)
	giveSpare(&s.bools, &sc.assocExisted)
	giveSpare(&s.bools, &sc.declaredOnlyBefore)
}

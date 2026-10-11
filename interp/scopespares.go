// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// scopeSpares holds the records of scopes that have closed, emptied, for the
// next scope or declaration to write into.
//
// A scope keeps what a declaration displaced in a record per name — see
// scoperecords.go — and each one is an allocation, as is the list that holds
// them. When the records were a table per kind, `local a` made about thirty
// allocations for one name (#6361); keeping the closed scope's records for
// the next call makes a declaration cost the writes it does and not the
// storage it writes into. This is the variable-store sibling of what #6100
// did for the option table.
//
// What a scope sees is unchanged: a scope still starts with no records, and
// is handed a spare only at the moment it would have made one.
//
// Per runner, and never shared: a clone starts with none (see ownTables), so a
// subshell running beside its parent cannot be handed a table the parent is
// writing. A popped scope is released only by the runner popping it, after
// everything that reads its tables has run.
type scopeSpares struct {
	records []*shadowRecord
	lists   [][]*shadowRecord
	// names is the list a return gathers the names to put back into. See
	// Runner.popScope, which takes it for the length of one unwind.
	names []string
}

// maxScopeSpares bounds each list. A call stack deeper than this holds more
// records at once than are kept, and the surplus is left to the collector, so
// one deep recursion does not pin its records for the life of the shell.
const maxScopeSpares = 256

// takeRecord is an empty record, a kept one where there is one.
func (s *scopeSpares) takeRecord() *shadowRecord {
	if n := len(s.records); n > 0 {
		rec := s.records[n-1]
		s.records[n-1] = nil
		s.records = s.records[:n-1]
		return rec
	}
	return new(shadowRecord)
}

// takeRecordList is an empty list to hold a scope's records.
func (s *scopeSpares) takeRecordList() []*shadowRecord {
	if n := len(s.lists); n > 0 {
		l := s.lists[n-1]
		s.lists[n-1] = nil
		s.lists = s.lists[:n-1]
		return l
	}
	return make([]*shadowRecord, 0, 4)
}

// releaseScopeTables hands the closed scope's records back for reuse, emptied
// so that nothing they held is kept alive by the list, and clears the scope's
// own fields so nothing reads a record through the closed scope.
func (r *Runner) releaseScopeTables(sc *scope) {
	s := &r.spares
	for i, rec := range sc.records {
		if len(s.records) < 4*maxScopeSpares {
			*rec = shadowRecord{}
			s.records = append(s.records, rec)
		}
		sc.records[i] = nil
	}
	if sc.records != nil && len(s.lists) < maxScopeSpares {
		s.lists = append(s.lists, sc.records[:0])
	}
	sc.records, sc.recordIndex = nil, nil
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"maps"
	"slices"
	"sync/atomic"
)

// The tables a clone shares with its parent until one of them writes.
//
// clonetables.go explains why a subshell owns every table: a write on one side
// must not reach the other, and a process substitution runs on a goroutine of
// its own, so one map written by two shells is a runtime fatal error rather
// than a leak. Owning by copying at the clone is the simple way to get both
// and it is the expensive one. Measured on the maintainer's real startup
// (#6306): about 100 clones per start, each copying every associative array
// (`_comps` alone is ~670 entries), the ~1,900-entry command hash and the
// ~1,000-entry function-origin table — about 85 MB of the 660 MB a start
// allocated, nearly all of it for a `$( … )` that wrote none of them. A
// control that did those copies twice cost +55 ms of the shell's own CPU per
// start, faster in 4 of 20 pairs.
//
// So the largest three are copied on write instead. A clone and its parent
// hold one table and one tableShare counting the runners holding it, and
// whichever writes first while another runner still holds it copies the table
// for itself and leaves the share. The rules, which are the whole of why this
// is safe across goroutines:
//
//   - **A table is written in place only by a runner that holds it alone**: a
//     nil share, or a count of one. A count of one cannot rise underneath the
//     check, because only a holder can clone, and the only holder is the
//     runner asking.
//   - **A runner that copies leaves the share only after the copy is made.**
//     Leaving first would let the last other holder see a count of one and
//     write the table while this copy is still reading it.
//   - **Reads never ask.** A shared table is never written by anyone, so any
//     number of goroutines may read it.
//   - **A finished clone gives its hold back** (releaseSharedTables), so that
//     the shell that ran a `$( … )` does not copy its own table on its next
//     write. A clone that never gives it back is still correct: the other
//     side copies once, as it used to at the clone.
//
// Every write to one of these tables goes through its own* method first; a
// write that does not is a write into a table the other shell may be reading.
// TestASubshellWriteToASharedTableStaysInTheSubshell holds one row for each.

// tableShare counts the runners holding one copy-on-write table.
type tableShare struct{ holders atomic.Int32 }

// shareTable records one more holder of the table behind *p, making the share
// if the table had none, and returns it for the clone to hold.
func shareTable(p **tableShare) *tableShare {
	if *p == nil {
		s := &tableShare{}
		s.holders.Store(1)
		*p = s
	}
	(*p).holders.Add(1)
	return *p
}

// mustCopyShared reports whether a runner about to write the table behind *p
// has to copy it first. When it does not, the runner now holds the table
// alone and *p is cleared.
func mustCopyShared(p **tableShare) bool {
	s := *p
	if s == nil {
		return false
	}
	if s.holders.Load() == 1 {
		*p = nil
		return false
	}
	return true
}

// leaveShare gives up this runner's hold, after a copy or when the runner is
// finished with the table.
func leaveShare(p **tableShare) {
	if s := *p; s != nil {
		*p = nil
		s.holders.Add(-1)
	}
}

// shareTables is ownTables' half for the copy-on-write tables: the clone
// already points at the parent's tables through `c := *r`, and both now hold
// them.
func (c *Runner) shareTables(r *Runner) {
	c.funcOriginsShare = shareTable(&r.funcOriginsShare)
	c.cmdHashShare = shareTable(&r.cmdHashShare)
	c.assocShare = shareTable(&r.assocShare)
}

// releaseSharedTables gives back a finished clone's hold on every shared
// table. The clone must not use them afterwards, and it lets go of them too,
// so that a write it made by mistake would land in a table of its own rather
// than in place in one the parent is now writing.
func (r *Runner) releaseSharedTables() {
	leaveShare(&r.funcOriginsShare)
	leaveShare(&r.cmdHashShare)
	leaveShare(&r.assocShare)
	r.funcOrigins, r.cmdHash, r.cmdHashOrder, r.cmdHashHits, r.AssocArrays = nil, nil, nil, nil, nil
}

// ownFuncOrigins makes the function-origin table this runner's to write.
func (r *Runner) ownFuncOrigins() {
	if mustCopyShared(&r.funcOriginsShare) {
		m := maps.Clone(r.funcOrigins)
		leaveShare(&r.funcOriginsShare)
		r.funcOrigins = m
	}
}

// ownCommandHash makes the command hash and its listing order this runner's
// to write. The order is a slice appended to in place, so it is copied with
// the table it lists; and the hits counted beside a shared table go into it.
func (r *Runner) ownCommandHash() {
	if mustCopyShared(&r.cmdHashShare) {
		m, order := maps.Clone(r.cmdHash), slices.Clone(r.cmdHashOrder)
		leaveShare(&r.cmdHashShare)
		r.cmdHash, r.cmdHashOrder = m, order
	}
	for name, n := range r.cmdHashHits {
		if e, ok := r.cmdHash[name]; ok {
			e.hits += n
			r.cmdHash[name] = e
		}
	}
	r.cmdHashHits = nil
}

// countSharedHit counts a hit on a remembered name without writing the table,
// where the table is still shared, and reports whether it did.
//
// Every lookup the table answers is a hit, so a `$( … )` that runs one
// external command would otherwise copy the whole table to add one to one
// entry — the commonest reason a substitution wrote it at all. The count is
// kept beside the table instead, and ownCommandHash adds it in the moment the
// table becomes this runner's to write. hashedHits is the reading.
func (r *Runner) countSharedHit(name string) bool {
	if s := r.cmdHashShare; s == nil || s.holders.Load() == 1 {
		return false
	}
	if r.cmdHashHits == nil {
		r.cmdHashHits = map[string]int{}
	}
	r.cmdHashHits[name]++
	return true
}

// hashedHits is how many lookups an entry has answered, counting the ones
// kept beside a shared table.
func (r *Runner) hashedHits(name string, e hashedCommand) int {
	return e.hits + r.cmdHashHits[name]
}

// replaceCommandHash puts a table this runner made in place of the one it
// held, which is not a write to the old one and so needs no copy of it. The
// hits counted beside the old table were the old table's.
func (r *Runner) replaceCommandHash(m map[string]hashedCommand, order []string) {
	leaveShare(&r.cmdHashShare)
	r.cmdHash, r.cmdHashOrder, r.cmdHashHits = m, order, nil
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

// dropAssocTable takes a name out of the associative arrays. A name that is
// not there is no write at all, and is not a reason to copy: assignments
// clear the name from every table on the way past, so most of these find
// nothing to take.
func (r *Runner) dropAssocTable(name string) {
	if _, ok := r.AssocArrays[name]; !ok {
		return
	}
	r.ownAssocs()
	delete(r.AssocArrays, name)
}

// replaceAssocs puts tables this runner made in place of the ones it held.
func (r *Runner) replaceAssocs(m map[string]AssocArray) {
	leaveShare(&r.assocShare)
	r.AssocArrays = m
}

// cloneAssocTables is a deep copy of the associative arrays, keeping a nil
// table nil.
func cloneAssocTables(in map[string]AssocArray) map[string]AssocArray {
	if in == nil {
		return nil
	}
	out := make(map[string]AssocArray, len(in))
	for k, v := range in {
		out[k] = v.clone()
	}
	return out
}

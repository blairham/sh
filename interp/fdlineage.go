// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "sync/atomic"

// Whose table a descriptor is in, when every shell this one runs shares one.
//
// A real shell forks each concurrent piece of a script — a substitution's
// body, a pipeline element, a background job — and each fork has a table of
// its own: what the parent held at the moment of the fork, and what the fork
// opens afterwards. Nothing a sibling opens later is in it, and nothing it
// opens is in the sibling's. Here all of them are goroutines over the one
// table the kernel gives the process, so a number a sibling's substitution
// is sitting on reads as taken to every other piece, and which piece gets the
// number it wanted is the scheduler's choice: `/bin/cat <(echo <(true)) <(echo
// <(true))` is `/dev/fd/10 /dev/fd/10` in zsh 5.9.2 every time, and was `10
// 12` or `11 10` here about one run in three on darwin (#6543).
//
// fdLineage is the fork tree, kept only as far as that question needs it. A
// runner's node is made where it is cloned, which is every place this shell
// stands in for a fork; the node remembers its parent and *when* it was made,
// on a clock every claim is stamped from. A claim on a descriptor is in a
// runner's table when it was made by that runner, or by an ancestor before
// the fork on the way down — the same two clauses a real fork's table obeys.
// See foreignfds_unix.go for the claims.
type fdLineage struct {
	parent *fdLineage
	// forkedAt is the clock reading when this node was made. A claim of the
	// parent's stamped at or after it was made after the fork, so the fork
	// does not hold it.
	forkedAt uint64
}

// fdClock orders forks and claims against each other. One counter for the
// process, because the question it answers — did this claim come before that
// fork — is asked across runners.
var fdClock atomic.Uint64

// lineage is this runner's node, made on first asking for the runner a
// script started in. Every clone is given its own in clone, so only a root
// ever reaches the allocation here.
func (r *Runner) lineage() *fdLineage {
	if r.fdNode == nil {
		r.fdNode = &fdLineage{forkedAt: fdClock.Add(1)}
	}
	return r.fdNode
}

// forkLineage gives a clone a node of its own under r's.
func (r *Runner) forkLineage() *fdLineage {
	return &fdLineage{parent: r.lineage(), forkedAt: fdClock.Add(1)}
}

// holds reports whether a claim made by owner at stamp is in the table of the
// runner whose node l is.
//
// A nil owner is a claim in nobody's table — a descriptor no forking shell
// would hold anywhere — and is held by nobody.
func (l *fdLineage) holds(owner *fdLineage, stamp uint64) bool {
	if owner == nil {
		return false
	}
	cut := ^uint64(0)
	for n := l; n != nil; n = n.parent {
		if n == owner {
			return stamp < cut
		}
		cut = n.forkedAt
	}
	return false
}

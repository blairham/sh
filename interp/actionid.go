// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strconv"
	"sync/atomic"
)

// An action's identity, which is the one thing the seam could not say.
//
// Every action is asked about once and then reported one to three times — a
// refusal, a start, an end, an access, an error — and until this there was
// nothing tying those records to each other or to the consultation that
// preceded them. Consumers paired them by ordering, and ordering is exactly
// what concurrency breaks: a background job and each half of a pipeline emit
// from their own goroutines, so a command's end is not the record after its
// start. A front end that fell back to matching on a fingerprint of the kind,
// the path and the arguments closed the wrong record whenever a script ran the
// same command twice at once.
//
// The id is a counter rather than a random string, and that is a cost decision
// with a correctness argument behind it. A PATH search stats a candidate in
// every directory PATH names and a glob stats every entry it descends past, so
// this is one of the hottest things the interpreter does; sixteen bytes of
// crypto/rand per stat would be paid by every shell that had merely asked to
// watch itself. What a counter gives up is uniqueness beyond one session, and
// Runner.Session is what restores it: the pair is unique everywhere, which is
// what the schema writes down.
//
// Nothing is numbered when nothing is watching. A Runner with no Gate and no
// Sink produces no records for an id to appear in, so it pays a nil check and
// not an allocation — the same fast path the probes in fsgate.go already take,
// for the same reason.

// ensureActionIDs gives this session its counter.
//
// Called from RunPart, beside the other ensures, which is the one point that
// is reliably before any clone and reliably single-threaded: a subshell or a
// background job copies whatever is here, and allocating lazily at the first
// action would let two goroutines allocate two counters and hand two actions
// the same id.
func (r *Runner) ensureActionIDs() {
	if r.actionIDs == nil {
		r.actionIDs = new(atomic.Uint64)
	}
}

// act stamps an action with its identity, and is how every Action in this
// package is built.
//
// Taken at construction rather than at the gate, because the value has to
// carry the id onwards: the gate is consulted with a copy, and the events that
// follow are built from the same variable the caller still holds. Stamping
// inside allowed() would give the gate an id nothing else ever saw.
//
// Atomic rather than mutex-guarded because it is called from every goroutine a
// shell has and does nothing else — the counter is the whole of the state.
func (r *Runner) act(a Action) Action {
	if r.actionIDs == nil || (r.Gate == nil && r.Events == nil) {
		return a
	}
	a.ID = strconv.FormatUint(r.actionIDs.Add(1), 10)
	return a
}

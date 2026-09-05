// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package boundary asks a shell's gate about the files a *front end* opens.
//
// The boundary is drawn around execution, and interp holds it for everything a
// script does. But a shell opens two kinds of file before and after any script
// runs — the program it was invoked with, the startup files it sources, the
// history it keeps — and those went through the os package directly. They were
// latent while nothing supplied a gate; they stopped being latent when
// driver.Shell grew Gate and Events, because a policy that refuses a path can
// now be handed to a binary that reads that path without asking.
//
// The rule for what belongs here is the one on interp's action vocabulary: an
// access is inside the boundary when the *path was chosen by whoever the
// policy is about*. A script operand, $ENV, HISTFILE — all of them come from
// the invocation or from the shell's own variables, which a line of script can
// set. The front end's own plumbing on fixed paths is outside it, and each
// such exemption is written down rather than left to be inferred; docs/design.md
// carries the list.
//
// The vocabulary is interp's and stays interp's. This package only asks: it
// does not name new kinds, does not decide what a refusal means, and does
// nothing at all when a shell has neither a gate nor a sink, which is the
// common case and costs two nil checks.
package boundary

import (
	"context"
	"time"

	"github.com/blairham/sh/internal/event"
	"github.com/blairham/sh/interp"
)

// Boundary is a shell's gate and event sink, as a pair, for the front end's
// own accesses. The zero value allows everything and records nothing, which
// is what a shell without a policy is.
type Boundary struct {
	Gate   interp.Gate
	Events interp.Sink
	// Session identifies the run these accesses belong to, and is the same
	// string the Runner carries — the front end hands one value to both.
	//
	// It matters here more than it looks. What this package opens is a shell's
	// *own* files, and one of them is the store that records what the shell
	// ran: without the session on these records, the audit stream's account of
	// the block store being written could not be joined to the blocks in it.
	Session string
}

// Open reports whether the front end may open path, recording it either way.
//
// A refusal is reported to the sink as EventDenied and nothing else happens:
// what a denied open *means* is the caller's, because the two callers mean
// different things by it. A script the shell cannot read is a failure it names
// and exits over; a startup file it cannot read is not a failure at all.
func (b Boundary) Open(ctx context.Context, path string, write bool) bool {
	a := interp.Action{ID: b.id(), Kind: interp.ActionOpen, Path: path, Write: write}
	if b.Gate != nil && b.Gate.Allow(ctx, a) == interp.Deny {
		b.emit(ctx, interp.Event{Kind: interp.EventDenied, Action: a})
		return false
	}
	// Recorded before the open rather than after it, unlike interp's, and
	// the difference is what there is to say afterwards: interp records a
	// *successful* open because a failed one already becomes an EventError
	// with the reason. Out here a failure is often not an error — a startup
	// file that is not there is the normal case — so the auditable act is
	// the attempt, which is what a policy was asked about.
	b.emit(ctx, interp.Event{Kind: interp.EventAccess, Action: a})
	return true
}

func (b Boundary) emit(ctx context.Context, e interp.Event) {
	if b.Events == nil {
		return
	}
	// The session is put on here for the reason interp puts it on in emit():
	// one place, so no call site can produce a record that belongs to nothing.
	e.Session = b.Session
	b.Events.Emit(ctx, e)
}

// id names one front-end access, so the consultation and the record of it are
// provably the same access — the promise interp.Action.ID makes, kept on this
// side of the boundary too.
//
// Minted rather than counted, which is the opposite of interp's choice and for
// the opposite reason. There a counter is forced by the hot path: a PATH search
// stats a candidate per directory. Out here a run makes a handful of these — a
// script, a startup file or two, a history file, a block store — so there is
// nothing to economize, and an id that needs no coordination is what lets the
// several places that build a Boundary go on building one independently. A
// shared counter would have to be threaded through every one of them, and the
// first that forgot would issue a duplicate.
//
// Nothing is minted when nothing is watching, which is the same nil check the
// rest of this package costs a shell without a policy.
func (b Boundary) id() string {
	if b.Gate == nil && b.Events == nil {
		return ""
	}
	return event.NewID(time.Now())
}

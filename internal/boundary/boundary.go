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

	"github.com/blairham/sh/interp"
)

// Boundary is a shell's gate and event sink, as a pair, for the front end's
// own accesses. The zero value allows everything and records nothing, which
// is what a shell without a policy is.
type Boundary struct {
	Gate   interp.Gate
	Events interp.Sink
}

// Open reports whether the front end may open path, recording it either way.
//
// A refusal is reported to the sink as EventDenied and nothing else happens:
// what a denied open *means* is the caller's, because the two callers mean
// different things by it. A script the shell cannot read is a failure it names
// and exits over; a startup file it cannot read is not a failure at all.
func (b Boundary) Open(ctx context.Context, path string, write bool) bool {
	a := interp.Action{Kind: interp.ActionOpen, Path: path, Write: write}
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
	b.Events.Emit(ctx, e)
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package boundary asks a shell's gate about what a *front end* does itself.
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
	"syscall"
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
	return b.ask(ctx, interp.Action{ID: b.id(), Kind: interp.ActionOpen, Path: path, Write: write})
}

// Exec reports whether the front end may run a program, recording it either
// way. Argv is the whole vector, argv[0] included, as interp builds one.
//
// The caller is an ACP client: an agent asking us to run a command, which is
// the one arrangement where a program a policy is about is named by somebody
// outside this process. It is squarely inside the boundary by this package's
// own rule — the program was chosen by whoever the policy is about — and it is
// the reason the rule is worth stating as a rule rather than as a list of the
// files a shell opens.
func (b Boundary) Exec(ctx context.Context, path string, argv []string) bool {
	return b.ask(ctx, interp.Action{ID: b.id(), Kind: interp.ActionExec, Path: path, Args: argv})
}

// Signal reports whether the front end may signal a process, recording it
// either way. The pid is as kill(2) takes it, so a negative value names a
// process group.
func (b Boundary) Signal(ctx context.Context, pid int, sig syscall.Signal) bool {
	return b.ask(ctx, interp.Action{ID: b.id(), Kind: interp.ActionSignal, PID: pid, Signal: sig})
}

// Record notes an access the front end made without asking about it.
//
// It is deliberately narrow, and the rule for reaching for it is this
// package's own: an act belongs here rather than at the gate when the *front
// end* chose it rather than whoever the policy is about. The worked example is
// an ACP client releasing a terminal — the protocol's only way for an agent to
// say it is finished, whose kill is the client ending something the client
// started. Everything the policy subject chose goes through the three above,
// which ask and record together.
//
// It stamps the id and the session for the same reason those do: a record that
// names no action and no run is a record nothing can be joined to, which is
// exactly what this field was added to fix.
func (b Boundary) Record(ctx context.Context, a interp.Action) {
	a.ID = b.id()
	b.emit(ctx, interp.Event{Kind: interp.EventAccess, Action: a})
}

// ask is the whole of the three above: consult, record, answer.
func (b Boundary) ask(ctx context.Context, a interp.Action) bool {
	if b.Gate != nil && b.Gate.Allow(ctx, a) == interp.Deny {
		b.emit(ctx, interp.Event{Kind: interp.EventDenied, Action: a})
		return false
	}
	// Recorded before the access rather than after it, unlike interp's, and
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

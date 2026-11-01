// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "context"

// runHandlersAsASubstitutionEnds runs the handlers waiting as a command
// substitution ends, in the dialect that runs them there rather than after the
// command the substitution is in. See Semantics.HandlersRunAsASubstitutionEnds.
//
// before is `$?` as it stood when the substitution began, which is what zsh
// shows a handler there: the status from before the substitution, where its
// handler between commands sees the one from before the command
// (Semantics.SignalHandlerSeesEarlierStatus). ksh93's sees the substitution's
// own, which is what Runner.status holds by now.
//
// Only the shell itself asks. A subshell's arrivals are the shell's to run
// once that subshell is done (Runner.runPendingTraps), and a substitution
// inside a subshell or inside another substitution ends while the shell is
// still waiting for the one around it.
//
// The common case costs a field read and an atomic load: nothing has arrived.
func (r *Runner) runHandlersAsASubstitutionEnds(ctx context.Context, before int) {
	if r.inSubshell {
		return
	}
	if s := r.signals; s == nil || s.nothingArrived() || !s.handlerWaitingFromElsewhere() {
		return
	}
	if !r.ask(r.sem().HandlersRunAsASubstitutionEnds,
		"when the handler of a signal that lands during a command substitution runs") {
		return
	}
	saved := r.statusBefore
	r.statusBefore = before
	r.runPendingTraps(ctx)
	r.statusBefore = saved
}

// handlerWaitingFromElsewhere reports whether a handler is waiting to run for
// an arrival the shell's own command substitution did not raise with `kill
// $$`: a signal from another process, or from one of the script's background
// jobs, which are other processes in every shell this models.
//
// A substitution's own `kill $$` is left out because the columns split on it
// for a reason of their own — zsh's substitution is a process of its own, so
// the signal comes from another process there, and ksh93's runs in the shell's
// own process and holds the handler until after the command — and that is not
// this question. A child's death is never one either, as for pendingTrap.
//
// The origins are a queue per condition and say nothing about which arrival
// on the list each belongs to, so the count decides: more arrivals of a
// condition waiting than its substitutions raised means one came from
// elsewhere.
func (s *signalState) handlerWaitingFromElsewhere() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.drainForwarded()
	for _, name := range s.pending {
		if name == "CHLD" || s.traps[name] == "" {
			continue
		}
		waiting, raised := 0, 0
		for _, other := range s.pending {
			if other == name {
				waiting++
			}
		}
		for _, o := range s.origins[name] {
			if o == originSubstitution {
				raised++
			}
		}
		if waiting > raised {
			return true
		}
	}
	return false
}

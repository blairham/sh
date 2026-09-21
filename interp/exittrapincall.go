// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "context"

// runExitTrapInsideTheExitingCall fires `trap … EXIT` where `exit` was
// written, rather than leaving it to Runner.Finish once the call it ran in
// has been unwound.
//
// The two orders are the same shell and a different *view*: the body runs
// either way, with the same status and the same output, and what changes is
// what it can see of the call it was fired from — the function's locals, its
// positional parameters, and whatever name the dialect gives the running
// function. See [Semantics.ExitTrapRunsInsideTheExitingCall] for the panel.
//
// Read at `exit` and not at the trap, because `exit` is the whole of what the
// axis is about: a function that falls off its end has returned before the
// shell ends, and every column's trap sees nothing there. Reaching for this
// in Runner.runExitTrap would have no call left to ask about.
//
// Only inside a **function** call. At the top level there is no stack to keep
// up, so the two readings cannot be told apart and the axis must not be
// asked: an unanswered one is a refusal, and refusing `exit` in a shell with
// a trap would be a refusal of the ordinary shape.
//
// A function frame and not any frame, which [Runner.InCall] would have
// answered: a script, a sourced file and a startup file are frames too, and
// `trap … EXIT; exit 3` at the top of a startup file is the ordinary shape
// this must not reach. Reading InCall refused exactly that, in the one
// arrangement where a shell reads a file before it has run anything.
func (r *Runner) runExitTrapInsideTheExitingCall(ctx context.Context) {
	if r.exitTrap == nil || !r.insideAFunctionCall() {
		return
	}
	if !r.ask(r.sem().ExitTrapRunsInsideTheExitingCall,
		"the EXIT trap running inside the call that exited") {
		return
	}
	// Runner.Finish reaches runExitTrap too, and finds nothing: the trap is
	// cleared on the way in, which is what keeps one body from running twice.
	r.runExitTrap(ctx)
}

// insideAFunctionCall reports whether any frame standing is a call of a
// function, as against the script, a sourced file or a startup file.
func (r *Runner) insideAFunctionCall() bool {
	for _, f := range r.CallStack() {
		if f.IsFunction() {
			return true
		}
	}
	return false
}

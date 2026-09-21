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
// Only inside a call. At the top level there is no stack to keep up, so the
// two readings cannot be told apart and the axis must not be asked: an
// unanswered one is a refusal, and refusing `exit` in a shell with a trap
// would be a refusal of the ordinary shape.
func (r *Runner) runExitTrapInsideTheExitingCall(ctx context.Context) {
	if r.exitTrap == nil || !r.InCall() {
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

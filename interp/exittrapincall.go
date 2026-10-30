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

// runExitTrapInsideTheExitingText fires `trap … EXIT` while the `eval` text
// or the sourced file a stop was raised in is still the place the shell is
// in, rather than leaving it to Runner.Finish once the shell has unwound to
// the top. Called by runSourcedText as the text stops, before any of what it
// set up — the text's name, its line numbering, a `.`'s operands — is put
// back. See [Semantics.ExitTrapRunsInsideTheExitingText] for the panel.
//
// At the end of the text and not at the `exit`, because the dialect that says
// yes leaves a function's frame first: a call standing below the stop is left
// whole, text it runs included, and the trap runs in the innermost text below
// it — so this is asked at each text the unwinding passes through, and the
// first one with no call standing under it is where the trap runs.
//
// Only for a stop the script asked for — `exit`, `set -e` firing, `${x?word}`
// in the dialect that makes it one, or a `return` with nothing left to return
// from. A signal's death, a canceled run and a fatal error are not measured
// here and keep the trap where it was.
func (r *Runner) runExitTrapInsideTheExitingText(ctx context.Context) {
	if r.exitTrap == nil || !r.stopLeavesTheShell() || r.insideAFunctionCall() {
		return
	}
	// The ends Runner.Finish and Runner.Run run no EXIT trap at all: running
	// it here would run one they decline.
	if r.canceledChunk || r.trapInterrupt || r.heldExitSkipsTheExitTrap {
		return
	}
	// And an `exit` in the EXIT trap's own body, which is already running
	// and would be entered again under itself.
	if r.inExitTrap {
		return
	}
	// The rest keep the trap where it was because nothing has measured them
	// here: an interactive shell, a startup file, and a shell that hangs up
	// its jobs as it leaves, which orders the hangup against the trap at the
	// end of the shell — see Semantics.HangupAtExitPrecedesTheExitTrap. A
	// signal trap's body is not among them: measured 2026-10-30 on zsh 5.9.2,
	// `trap 'eval "exit 0"' USR1` and `trap '. ./inc.sh' USR1` fired from
	// line 6 locate the EXIT trap at `(eval):6` and `./inc.sh:2`.
	if r.Interactive || r.startupDepth > 0 || r.hangUpJobsAtExit && r.accountsForJobsAtExit() {
		return
	}
	if !r.ask(r.sem().ExitTrapRunsInsideTheExitingText,
		"the EXIT trap running inside the `eval` text or sourced file that stopped") {
		return
	}
	// The stop goes on unwinding afterwards exactly as it was, so the
	// constructs it still passes through see the same stop: a try block's
	// cleanup half reads whether this was `set -e`, and a `return` is still
	// a `return` to the subshell it ends. A body that exited replaces it, as
	// it would have at the end of the shell.
	ctl, abandon, errexit := r.ctl, r.abandon, r.errexitStopped
	if !r.runExitTrap(ctx) {
		r.ctl, r.abandon, r.errexitStopped = ctl, abandon, errexit
	}
}

// stopLeavesTheShell reports whether the stop being carried is one the script
// raised and nothing between here and the top will catch: an `exit`, `set -e`
// firing or a `${x?word}`, or a `return` with no function and no sourced file
// left to return from.
func (r *Runner) stopLeavesTheShell() bool {
	switch r.ctl {
	case controlExit:
		return r.exitRan || r.errexitStopped || r.abandon == abandonParamError
	case controlReturn:
		return r.sourceDepth == 0
	}
	return false
}

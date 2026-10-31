// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// heldExitTrap is the EXIT trap a function call set aside at its entry, in
// the shell that holds the caller's handler for the length of a body — see
// Semantics.ExitTrapIsFunctionLocal and the entry of Runner.callFuncInPlace.
//
// Every field the call puts back at an ordinary return, together: the action,
// the depth and the scoping answer it was set under, and the `TRAPEXIT`
// function it may be spelled as. A put-back that missed one would leave a
// handler standing under the wrong scoping, which is the kind of half-restore
// the return's own branches already avoid by writing all of them at once.
type heldExitTrap struct {
	trap  *string
	depth int
	local Answer
	fn    string
	decl  *syntax.FuncDecl
}

// heldExitTrapIsLost reports whether a call now being left gives up the EXIT
// trap it held instead of putting it back, because what is unwinding it is a
// stop that ends the shell from inside the call.
//
// Three producers answer it through the axis — `set -e` firing, `${x?word}`
// firing, and an untrapped HUP the dialect ends on as an exit, whether the
// shell sent it itself or another process did (Runner.hungUp); see
// Semantics.StopInsideACallLosesTheHeldExitTrap — and a fourth without it: an `exec` that could not run, where the dialect has already
// said the EXIT trap is dropped (Semantics.ExecFailureRunsExitTrap and the
// pathname field beside it). A dropped trap is dropped wherever it is
// standing, and a held one is standing in the caller. Measured 2026-10-30 on
// zsh 5.9.2, `trap 'echo T' EXIT; f() { exec /nonexistent/x; }; f` writes the
// complaint and no T, as the same `exec` at the top level does.
//
// An error the shell reported is not on the list. It unwinds through the call
// like any other transfer and the call puts the trap back on the way: with a
// division by zero, a readonly reassignment or an unset name under `set -u`
// in place of the `false`, zsh writes the complaint and then T. Nor is
// `exit`, which runs the trap itself.
func (r *Runner) heldExitTrapIsLost() bool {
	if r.ctl != controlExit {
		return false
	}
	if r.execDroppedTheExitTrap {
		return true
	}
	if !r.errexitStopped && r.abandon != abandonParamError && !r.hungUp {
		return false
	}
	return r.sem().StopInsideACallLosesTheHeldExitTrap == Yes
}

// loseTheHeldExitTrap records a held trap a call is not putting back, so that
// something catching the stop further up can put it back after all.
//
// The outermost call to be left is the last to write it, and that is the one
// an ordinary unwinding would have left standing at the top, so a later
// write replacing an earlier one is the rule rather than an accident: an inner
// call's held trap is the outer call's own, which does not outlive the outer
// call whichever way it is left.
func (r *Runner) loseTheHeldExitTrap(h heldExitTrap) {
	r.lostExitTrap = &h
}

// putBackALostExitTrap is the other half: a boundary that catches the stop —
// a typed line given up, a trap handler's error ending the handler, a file
// given up — means the shell is not ending after all, and the trap the calls
// held comes back as their returns would have brought it.
//
// Measured 2026-10-30 at a prompt through a pseudo-terminal, zsh 5.9.2 under
// `-f -i`: after `trap 'echo T' EXIT` and `f() { : ${q?b}; }`, typing `f`
// writes `f: q: b` and draws the next prompt, `trap` still lists the handler,
// and `exit` writes T. errexit in the same place is never caught — the
// session ends there with no T — so only the parameter error ever reaches
// here.
func (r *Runner) putBackALostExitTrap() {
	h := r.lostExitTrap
	if h == nil {
		return
	}
	r.lostExitTrap = nil
	r.exitTrap, r.trapDepth, r.exitTrapLocal = h.trap, h.depth, h.local
	r.restoreTheExitTrapFunction(h.fn, h.decl)
}

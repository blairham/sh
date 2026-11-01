// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"sync/atomic"
	"syscall"
)

// A fatal signal from outside that a `( … )` or `$( … )` the shell runs in its
// own process waits out.
//
// One shell runs both in its own process rather than in a child, and catches
// every fatal signal once an EXIT trap has been set. A signal that lands while
// such a subshell waits for a program does not end the wait: the program runs
// to its end, so does the rest of the subshell, and the death is the shell's
// once the subshell is over. Measured 2026-10-31 on ksh93u+ 2012-08-01, each
// shell in a session of its own, `/bin/kill` from another process 0.4 s in,
// `trap 'echo "bye $?"' EXIT; (exit 7)` in front of each line:
//
//	( /bin/sleep 2; echo in ); echo after      in, bye 0, killed at 2.0 s
//	( /bin/sleep 2; exit 3 ); echo after       bye 3, killed at 2.0 s
//	x=$(/bin/sleep 2; exit 3); echo after      bye 3, killed at 2.0 s
//	/bin/sh -c '…; echo got' "$(/bin/sleep 1)"  `got` at 2.0 s, then bye 0
//	: | ( /bin/sleep 2; echo in ); echo after  in, bye 0, killed at 2.0 s
//	( /bin/sleep 2 | /bin/cat; echo in ); …    in, bye 0, killed at 2.0 s
//	{ /bin/sleep 2; echo in; }; echo after     bye 7, killed at 0.4 s
//
// for TERM, HUP, USR1, ALRM, QUIT and INT alike. bash 5.3.20 dies at 0.4 s on
// the first three with `bye 7`, as zsh 5.9.2 and dash do with nothing written:
// each runs the subshell in a child the signal never reaches, and leaves it
// running. bash 3.2.57 waits for the substitution and not for the
// parentheses, which no dialect here models. Without an EXIT trap ksh93 dies
// at 0.4 s too; with one set and then taken away it waits, writes `in`, and
// dies with nothing run — caught means an EXIT trap was set, ever, as for
// SignalDeathEndsAtAKeywordFunctionCall.
//
// The rest of the rule, each row measured the same day in ksh93:
//
//   - **A builtin is reached at once.** A busy loop or a `read` inside the
//     parentheses ends them where the signal lands, as a command killed by
//     it: `bye 271`, at 0.4 s.
//   - **The subshell around it dies of it where it ends.** `( ( /bin/sleep 2;
//     echo inner ); echo outer )` writes `inner` and `bye 271`: the outer
//     parentheses end as killed the moment the inner ones are over, `outer`
//     unwritten, and `( x=$(/bin/sleep 2); echo outer )` is the same.
//   - **A later program lets it go, and so does a later subshell.** `(
//     /bin/sleep 1; echo a; /bin/sleep 0.1; echo b ); echo after` writes all
//     three and exits 0; so does a `( … )` or a `$( … )` in place of the
//     second `sleep`, of builtins alone or not. A builtin or a function call
//     there does not, and the shell dies as the parentheses end.
//   - **A copy it forks is never reached.** A pipeline element but the last
//     runs to its end, busy loop or not, and the subshell waits for it. And a
//     subshell that starts a background job is forked there and then, so the
//     shell dies at once: `( /bin/sleep 2 & wait $!; echo in )` is `bye 7`
//     at 0.4 s, with `in` written by the orphan at 2.0 s.
//   - **A second signal is the first.** Another TERM 0.4 s later still
//     waits the subshell out.
//
// Not modeled: a `$( … )` that starts a background job is forked the same
// way and yet waited out — `x=$(/bin/sleep 2 & wait $!; echo in >&2)` writes
// `in` and `bye 7` at 2.0 s, where this shell dies at 0.4 s — and ksh93 loses
// some of a builtin's output after a substitution that held a death: `echo
// "got $(/bin/sleep 2; echo in)"` writes `got in` without its newline and
// reads `bye 271`, and `printf` writes nothing.
//
// An INT is held by a route of its own while the program runs, because it is
// held with or without an EXIT trap and for the program's sake: see
// foregroundinterrupt.go. Still pending as the parentheses end, it becomes the
// death they hold, so the rule above decides what the trap reads. Measured
// 2026-11-01 on ksh93u+, INT from another process 0.4 s in, the same prefix:
//
//	( /bin/sleep 2; echo in ); echo after      in, bye 0, killed by INT
//	( /bin/sleep 2; exit 3 ); echo after       bye 3, killed by INT
//	( ( /bin/sleep 2; echo inner ); echo outer ); echo after
//	                                           inner, bye 258, killed by INT
//
// where this shell's ksh read `bye 130` in all three (#7159).
//
// # The route
//
// A wait for a program inside such a subshell selects on the death being
// handed over as any wait does, and holds it rather than giving up: it takes
// it from the front end — so the watch stops waiting — and puts a fresh
// channel in its place, so that nothing else gives a wait up for a death
// that is already somebody's. The subshell ends as it would have, and its
// end hands the death on: to the subshell around it, which is ended as
// killed, or to the shell at the top, which takes it at its next command as
// it takes one from the front end — reading the status the subshell left,
// since nothing gave a wait up. A copy the subshell forks is out of the
// signal's reach, as a background job is: its waits are given up for
// nothing and it never stops at a command for the death.

// unforkedSubshell is one `( … )` or `$( … )` the shell runs in its own
// process, in the dialect that waits a death out for one.
type unforkedSubshell struct {
	// outer is the one around it, nil at the top.
	outer *unforkedSubshell
	// depth counts it and those around it, from one.
	depth int
	// forked says it has started a background job, which forks it in that
	// dialect: from then on it is a child of the shell's like any other.
	forked atomic.Bool
}

// enterAnUnforkedSubshell makes sub — a `( … )` or `$( … )` r is about to run
// in its own process — one that waits a death from outside out, where the
// dialect does and a front end hands deaths over, and hands back what to call
// once sub is over. A subshell inside a copy the shell forks is not one: the
// signal never reaches it. Nor is one in a shell that has never set an EXIT
// trap, which catches nothing to hold.
func (r *Runner) enterAnUnforkedSubshell(sub *Runner) (leave func()) {
	s := r.signals
	if s == nil || !r.caughtByAnExitTrap || r.Interactive || !s.outside.heard.Load() ||
		!r.inTheShellsOwnProcess() || r.sem().SignalDeathWaitsForAnUnforkedSubshell != Yes {
		return func() {}
	}
	depth := 1
	if r.unforked != nil {
		depth = r.unforked.depth + 1
	}
	sub.unforked = &unforkedSubshell{outer: r.unforked, depth: depth}
	s.outside.inSubshells.Add(1)
	return func() { s.outside.inSubshells.Add(-1) }
}

// inTheShellsOwnProcess reports whether r stands for the shell's own process
// rather than for a child it forks: not a pipeline element but the last, a
// background job or a process substitution's body, nor anything inside one.
func (r *Runner) inTheShellsOwnProcess() bool {
	return !r.midPipeline && r.bg == nil && r.pipeEnd == nil
}

// heldBySubshells reports whether r is inside a subshell that waits a death
// out, and every one around it is still unforked.
func (r *Runner) heldBySubshells() bool {
	if r.unforked == nil {
		return false
	}
	for u := r.unforked; u != nil; u = u.outer {
		if u.forked.Load() {
			return false
		}
	}
	return true
}

// inAnUnforkedSubshell reports whether r is the shell's own process inside a
// subshell that waits a death out: its waits for a program hold one, and a
// builtin of its ends the subshell as killed.
func (r *Runner) inAnUnforkedSubshell() bool {
	return r.unforked != nil && r.inTheShellsOwnProcess() && r.heldBySubshells()
}

// outOfADeathsReach reports whether r stands for a child such a subshell
// forked, which the signal never reaches: it gives up no wait for a death and
// never stops for one.
func (r *Runner) outOfADeathsReach() bool {
	return r.unforked != nil && !r.inTheShellsOwnProcess() && r.heldBySubshells()
}

// aSubshellWaitsOutADeath reports whether the shell at the top, with no EXIT
// trap standing, still has a subshell's end to hand a death over for: it has
// caught every fatal signal since it first set one, in the dialect that waits
// a death out for a subshell. Read and not asked, as the front end's watch
// reads it.
func (r *Runner) aSubshellWaitsOutADeath() bool {
	return r.caughtByAnExitTrap && r.sem().ExitTrapRunsOnSignalDeath == Yes &&
		r.sem().SignalDeathWaitsForAnUnforkedSubshell == Yes
}

// holdTheDeath is a wait for a program inside such a subshell holding the
// death it was handed rather than giving up for it, and reports whether the
// death is held: false where the front end took it back, or where it is not
// one the subshell could hold.
func (r *Runner) holdTheDeath() bool {
	s := r.signals
	o := &s.outside
	s.mu.Lock()
	defer s.mu.Unlock()
	if o.state.CompareAndSwap(outsidePosted, outsideHeld) {
		// Taken, so the front end stops waiting, and a fresh channel for
		// the waits that come after, which are not to give up for it.
		close(o.claimed)
		o.dying = make(chan struct{})
		o.heldDepth = r.unforked.depth
		return true
	}
	return o.state.Load() == outsideHeld
}

// holdsAnInterruptAtItsEnd is the end of r, such a subshell's `( … )` body,
// with an interrupt still pending from a program inside it (see
// Runner.interruptAtParenthesesEnd): it is held as the death r holds, for
// takeOverAHeldDeath to hand on as it hands on any other. It reports whether
// it is held so; false leaves the interrupt to end the shell as r ends, which
// is where no EXIT trap was ever set, or a death is already handed over.
func (r *Runner) holdsAnInterruptAtItsEnd() bool {
	if !r.inAnUnforkedSubshell() {
		return false
	}
	s := r.signals
	o := &s.outside
	s.mu.Lock()
	defer s.mu.Unlock()
	if o.state.Load() != outsideNone {
		return false
	}
	// As holdTheDeath leaves one the front end handed over: taken, so a
	// later signal from outside is taken as this one.
	o.name, o.sig = "INT", syscall.SIGINT
	o.state.Store(outsideHeld)
	close(o.claimed)
	o.heldDepth = r.unforked.depth
	return true
}

// aProgramLetsAHeldDeathGo is a program about to be waited for inside such a
// subshell, which lets go of a death held in it: the shell carries on as if
// none had landed, and the next one is handed over as the first was.
func (r *Runner) aProgramLetsAHeldDeathGo() {
	if r.unforked == nil || !r.inAnUnforkedSubshell() {
		return
	}
	s := r.signals
	if s.outside.state.Load() != outsideHeld {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outside.state.Load() == outsideHeld {
		s.outside.reopen()
	}
}

// takeOverAHeldDeath is the end of sub, a subshell r ran, with a death held:
// held inside sub, it is r's now — ending r at once as killed where r is the
// subshell around it, and due at the next command where r is the shell's own
// process at the top. Held in a subshell around sub before sub began, sub's
// ending lets go of it, as a later program would. It reports whether r has
// been ended.
func (r *Runner) takeOverAHeldDeath(sub *Runner) bool {
	u := sub.unforked
	if u == nil || u == r.unforked {
		return false
	}
	s := r.signals
	o := &s.outside
	if o.state.Load() != outsideHeld ||
		!r.ask(r.sem().SignalDeathWaitsForAnUnforkedSubshell,
			"a fatal signal from outside waiting for a subshell the shell runs in its own process") {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case o.state.Load() != outsideHeld:
		return false
	case o.heldDepth < u.depth:
		o.reopen()
		return false
	case r.inAnUnforkedSubshell():
		o.heldDepth = r.unforked.depth
		r.status = r.signalDeathStatus(o.sig)
		r.ctl, r.abandon, r.errexitStopped = controlExit, abandonRequested, false
		return true
	}
	o.state.Store(outsideDue)
	return false
}

// startsAJob is r starting a background job, which forks the unforked
// subshell r is running in, in the dialect that waits a death out for one.
func (r *Runner) startsAJob() {
	if r.unforked != nil && r.inTheShellsOwnProcess() {
		r.unforked.forked.Store(true)
	}
}

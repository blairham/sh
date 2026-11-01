// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "syscall"

// A fatal signal that ends a `function name { … }` call rather than the shell.
//
// One shell catches every fatal signal once an EXIT trap has been set, so that
// it can run the trap on a death; and in that shell a `function name { … }`
// call is a boundary the death stops at. The call ends where the signal
// landed, it answers the status of a command killed by the signal, its own
// EXIT trap runs as it returns, and the caller goes on. Measured 2026-10-31
// on ksh93u+ 2012-08-01, each shell in a session of its own, TERM from
// `/bin/kill` in another process half a second in, with
// `trap 'echo "bye $?"' EXIT` in front of each line:
//
//	function f { /bin/sleep 3; echo inf; }; f; echo "after $?"     after 271, bye 0, exit 0
//	function f { while :; do :; done; }; f; echo "after $?"        after 271, bye 0, exit 0
//	the same, `kill -TERM $$` in the body in place of the sleep    after 271, bye 0, exit 0
//	f() { /bin/sleep 3; echo inf; }; f; echo "after $?"            bye 271, killed by TERM
//	function f { …; }; f        (the call is the last command)     bye 271, killed by TERM
//
// bash 5.3.20 and 3.2.57 write `bye 0` and die of TERM for every one of the
// first three, which is SignalDeathEndsAtAKeywordFunctionCall. HUP, USR1,
// USR2, ALRM, PIPE, VTALRM, PROF, XCPU and XFSZ from another process end the
// call as TERM does, at 256 plus their number, and so do HUP, USR2, ALRM and
// PIPE sent with `kill $$`.
//
// The rest of the rule, each row measured the same day in ksh93:
//
//   - **Caught means an EXIT trap was set, ever.** Without one the shell dies
//     of the signal. With one set and then taken away again, set to `''`, or
//     set inside an earlier `function` call and fired at its return, the call
//     still ends: the shell went on catching. A trap on some other signal
//     does not do it.
//   - **Or a trap on the signal itself, ever.** Then the call ends on that
//     signal, and the caller's handler does not run: inside the call the
//     trap table is the call's own. The rows below, with the same `after
//     271` for a trap set and taken away again before the call — `trap
//     'echo t' TERM; trap - TERM` — or set to `''` and taken away, or set
//     inside an earlier `function` call; for an ignore the call itself takes
//     away; and for `kill -TERM $$` in place of the `sleep`. A `trap - TERM`
//     with nothing set before it leaves the shell dying of TERM, and so does
//     each of these with the POSIX-form call, where the caller's handler
//     runs if it still stands (#7156).
//
// Those rows, measured 2026-11-01 with no EXIT trap anywhere, the second
// with TERM from another process half a second in:
//
//	trap 'echo O' USR1; function q { echo mark1; kill -USR1 $$; echo mark2; }
//	q; echo "mark3 $?"; kill -USR1 $$; echo end
//	        mark1, mark3 286, O, end, exit 0
//	trap 'echo term' TERM; function f { /bin/sleep 3; echo inf; }
//	f; echo "after $?"
//	        after 271, exit 0
//
// And the rest of the rule:
//
//   - **The innermost `function` call is the one that ends.** A POSIX-form
//     call inside it is not a boundary — `g() { /bin/sleep 3; }; function f {
//     g; echo inf; }` ends at `f` with `inf` unwritten — and a keyword call
//     inside a POSIX one ends and lets the POSIX one go on.
//   - **The call's own EXIT trap runs, reading what the shell held.** `function
//     f { trap 'echo "fexit $?"' EXIT; (exit 5); /bin/sleep 3; }` writes
//     `fexit 5` and the call answers 271. Where the death landed in a
//     POSIX-form call inside it, the trap reads 271, which that call ended
//     at. An `exit` in it ends the shell; a `return` in it returns from the
//     caller, as any `return` written there does.
//   - **INT is not one of them.** It ends the shell from inside the call, as
//     it does from anywhere.
//   - **`kill` finishes its operands first**, as it does wherever the shell
//     has caught its own signal: `kill -TERM $$ 99999999` writes the
//     complaint about the second and then ends the call.
//
// A copy of the shell — a `( … )`, a pipeline element — is left as it was. In
// a reference it is a process of its own that a signal aimed at the shell
// never reaches, and here it stops where it stands for a death from outside
// and its parent takes the death (see outsidedeath.go), so a call in the copy
// is not the one that ends.
//
// An ignored signal stays ignored inside the call, which is the call's trap
// table keeping the caller's ignore: see localtraps.go (#7157).
//
// Not modeled: in ksh93 a trap set inside a `( … )` has the shell catching
// the signal too, since the parentheses run in the shell's own process; and
// INT, with a trap for it set before the call, runs the caller's handler and
// then ends the call at 258.

// takenDeath is where the death the shell is unwinding for was taken — the
// depth of the calls standing — and the status the shell held there.
type takenDeath struct {
	depth int
	held  int
}

// signalEndsAKeywordCall reports whether a fatal signal by this name is one
// that ends a `function name { … }` call in the dialect where any does: a
// named, catchable signal other than INT.
func signalEndsAKeywordCall(name string, sig syscall.Signal) bool {
	return name != "" && name != "INT" && catchableSignal(sig)
}

// signalSet is a set of signals by number, a bit each. A signal past the
// last bit is never in a set built by adding to one, and is in everySignal.
type signalSet uint64

// everySignal is the set that holds them all, which is what an EXIT trap ever
// set has the shell catching.
const everySignal = ^signalSet(0)

// with is the set with sig added.
func (ss signalSet) with(sig syscall.Signal) signalSet {
	if sig <= 0 || sig >= 64 {
		return ss
	}
	return ss | 1<<uint(sig)
}

// has reports whether sig is in the set.
func (ss signalSet) has(sig syscall.Signal) bool {
	if ss == everySignal {
		return true
	}
	return sig > 0 && sig < 64 && ss&(1<<uint(sig)) != 0
}

// caughtSignalSet is the signals the shell has caught: every one where an
// EXIT trap has been set, in the dialect that catches them to run it on a
// death, and otherwise each one it has ever had a trap for.
func (r *Runner) caughtSignalSet() signalSet {
	if r.caughtByAnExitTrap && r.sem().ExitTrapRunsOnSignalDeath == Yes {
		return everySignal
	}
	return r.caughtSignals
}

// signalsAKeywordCallEndsOn is the signals whose death would end a standing
// `function name { … }` call rather than the shell: the caught ones, where
// such a call is standing in the shell at the top and the dialect puts a
// boundary there. Read and not asked, as the front end's watch reads it: an
// unanswered axis is no, which leaves the signal to end the shell.
func (r *Runner) signalsAKeywordCallEndsOn() signalSet {
	if r.keywordCalls == 0 || r.inSubshell || r.sem().SignalDeathEndsAtAKeywordFunctionCall != Yes {
		return 0
	}
	return r.caughtSignalSet()
}

// aDeathWouldEndAKeywordCall is signalsAKeywordCallEndsOn for one signal.
func (r *Runner) aDeathWouldEndAKeywordCall(name string, sig syscall.Signal) bool {
	return signalEndsAKeywordCall(name, sig) && !r.segvEndsTheShellWhereItLands(name) &&
		r.signalsAKeywordCallEndsOn().has(sig)
}

// endTheCallOnADeath is the boundary, at the return of a `function name { … }`
// call at depth: where a fatal signal the shell caught is what is unwinding
// it, and the dialect ends the call there, the shell stops dying. It reports
// the signal, and leaves `$?` at what the call's own EXIT trap reads; the
// caller makes it the call's status once the trap has run.
//
// Asked only once there is a caught death to end, so a call that returns in
// the ordinary way asks nothing.
func (r *Runner) endTheCallOnADeath(depth int) syscall.Signal {
	sig := r.killedBySig
	if sig == 0 || r.ctl != controlExit || r.inSubshell || !r.caughtSignalSet().has(sig) ||
		!signalEndsAKeywordCall(r.killedBy, sig) || r.segvEndsTheShellWhereItLands(r.killedBy) ||
		!r.ask(r.sem().SignalDeathEndsAtAKeywordFunctionCall,
			"a fatal signal ending at the call of a `function` body") {
		return 0
	}
	shown := r.deathTaken.held
	if r.deathTaken.depth != depth {
		// Taken in a POSIX-form call inside this one, which ended as killed
		// on the way here.
		shown = r.signalDeathStatus(sig)
	}
	r.killedBy, r.killedBySig, r.stoppedBySignal = "", 0, false
	r.ctl, r.status = controlNone, shown
	r.exitTrapShownStatus = nil
	r.reopenTheHandover()
	return sig
}

// reopenTheHandover makes a death from outside that a call ended the last one
// handed over rather than the only one, so the next is handed over as the
// first was. Under the lock the front end's closure posts under, so a post
// can never see half of it.
func (r *Runner) reopenTheHandover() {
	s := r.signals
	if s == nil || s.outside.state.Load() != outsideClaimed {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outside.reopen()
}

// reopen puts the handover back where it started, with fresh channels for the
// next death. Called with the signal state's lock held.
func (o *outsideDeath) reopen() {
	o.dying, o.claimed = make(chan struct{}), make(chan struct{})
	o.abandoned.Store(false)
	o.state.Store(outsideNone)
}

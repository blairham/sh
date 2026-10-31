// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "syscall"

// A fatal signal the shell sends itself with an EXIT trap standing to run for
// it, and the `$?` that trap reads.
//
// Every shell with no EXIT trap to run, and zsh, dash and BusyBox ash with
// one, dies of `kill -TERM $$` as it sends. The two that run the trap on a
// death have caught the signal instead, and that changes what `kill` is: it
// finishes, sending to the operands after `$$` and complaining about the ones
// it cannot, and the shell dies afterwards. Measured 2026-10-30, each shell in
// a process group of its own, `trap 'echo "bye $?"' EXIT; (exit 7)` and then
// the line on the left:
//
//	                                bash 5.3   bash 3.2   ksh93
//	kill -TERM $$                   bye 0      bye 0      bye 7
//	! kill -TERM $$                 bye 1      bye 1      bye 7
//	kill -TERM $$ && echo and       bye 0      bye 0      bye 7
//	false; kill -TERM $$            bye 0      bye 0      bye 1
//	! { kill -TERM $$; }            bye 1      bye 1      bye 7
//	! { kill -TERM $$; echo in; }   bye 0      bye 0      bye 7
//	f() { (exit 7); kill …; }; f    bye 0      bye 0      bye 271
//	(kill -TERM $$)                 bye 7      bye 7      bye 271
//	x=$(kill -TERM $$)              bye 7      bye 7      bye 271
//	true && (kill -TERM $$)         bye 0      bye 0      bye 271
//	kill -TERM $$ 99999999          bye 0      bye 0      bye 7, both with the complaint
//
// with HUP and USR1 the same as TERM, bash called `sh` the same as bash, and
// nothing after the `kill` run in any row. This shell wrote `bye 143` in every
// one, the death's own status, which no reference shows.
//
// bash takes the death at its next command, the way it takes a signal from
// another process: whatever `kill` and the `!` around it left is what the
// trap reads, and in `! { kill; echo in; }` the next command is `echo in`, so
// the outer `!` never comes. ksh93 takes it as `kill` returns, before kill's
// status is recorded, so the trap reads what the shell held as `kill` ran:
// the status before it, or a substitution's in its operands. That is
// Semantics.SelfAimedDeathIsTakenAtTheNextCommand.
//
// A copy that sends it — a `( … )`, a `$( … )`, a pipeline element — is the
// same question, because in bash the copy is a process of its own and the
// shell above it is waiting: it stops waiting, and reads what it held before
// the command that made the copy. ksh93 runs the copy in its own process, so
// the signal ends the copy at 271, and that is the status the shell holds
// when it takes the death.
//
// And inside a function ksh93 reads 271 whichever way the death came — the
// shell's own `kill`, a copy's, or `/bin/kill` from another process — where
// bash reads what it read at the top. That is
// Semantics.DeathInAFunctionEndsTheCallAsKilled.
//
// Two rows are not modeled. bash 5.3 also takes the death as an `eval` or a
// `.` file ends, so `! eval 'kill -TERM $$'` is `bye 0` there and `bye 1` in
// bash 3.2 and here; and ksh93 reads the bare number, 15, after a `.` file
// sent it. ksh93 reads 0 for `false | kill -TERM $$`, too, where this shell
// reads what it held before the `kill`. See #7146.

// heldDeath is a fatal signal the shell sent itself and has not yet taken.
type heldDeath struct {
	name string
	sig  syscall.Signal
}

// exitTrapRunsOnThisDeath reports whether a fatal signal by this name would
// run the shell's EXIT trap rather than end it where it lands: there is a
// trap, the signal can be caught and has a name, the dialect runs the trap on
// a death, and it is not the SEGV the dialect leaves to kill where it lands.
//
// Read and not asked, as the front end's watch reads it: an unanswered axis is
// asked where the trap would run, and here it is no, which keeps the death
// where it was.
func (r *Runner) exitTrapRunsOnThisDeath(name string, sig syscall.Signal) bool {
	return r.exitTrap != nil && name != "" && catchableSignal(sig) &&
		r.sem().ExitTrapRunsOnSignalDeath == Yes && !r.segvEndsTheShellWhereItLands(name)
}

// holdTheSelfAimedDeath keeps a fatal signal the shell at the top just sent
// itself for the shell to take later, where its EXIT trap stands to run on
// it, and reports whether it did. A second one changes nothing: a process
// killed twice died of the first.
//
// Not inside the EXIT trap itself, where a fatal signal is a second death and
// ends the shell, or the trap, at once. See exittrapdeath.go.
func (r *Runner) holdTheSelfAimedDeath(name string, sig syscall.Signal) bool {
	if r.inSubshell || r.inExitTrap || !r.exitTrapRunsOnThisDeath(name, sig) {
		return false
	}
	if r.selfAimedDeath.sig == 0 {
		r.selfAimedDeath = heldDeath{name: name, sig: sig}
	}
	return true
}

// takeTheSelfAimedDeathAsKillReturns takes a death `kill` has just held, in
// the dialect that takes it there, and reports whether it did. The trap reads
// what the shell held as `kill` ran — kill's own status is never recorded —
// which is the status before it, or a substitution's in its operands:
// `(exit 7); kill -TERM $(exit 4) $$` is `bye 4` in ksh93, measured
// 2026-10-30.
func (r *Runner) takeTheSelfAimedDeathAsKillReturns() bool {
	if r.selfAimedDeath.sig == 0 || r.ask(r.sem().SelfAimedDeathIsTakenAtTheNextCommand,
		"where a fatal signal the shell sent itself is taken") {
		return false
	}
	return r.takeTheSelfAimedDeath()
}

// takeTheSelfAimedDeath takes a held death — at the next command, where the
// input runs out before one, or as `kill` returns — and reports whether the
// shell is stopping. The trap reads what the shell holds there.
func (r *Runner) takeTheSelfAimedDeath() bool {
	d := r.selfAimedDeath
	if d.sig == 0 {
		return false
	}
	r.selfAimedDeath = heldDeath{}
	shown := r.status
	r.signalDeath(d.name, d.sig)
	r.showTheExitTrap(shown, d.name, d.sig)
	return r.ctl != controlNone
}

// copysSelfAimedDeathShows is the status the EXIT trap reads for a death a
// copy's `kill` sent the shell: in the dialect that takes the shell's own at
// the next command, what the shell held when it made the copy, and otherwise
// what it holds now that the copy is done.
func (r *Runner) copysSelfAimedDeathShows(d sharedDeath) int {
	if !r.exitTrapRunsOnThisDeath(d.name, d.sig) ||
		!r.ask(r.sem().SelfAimedDeathIsTakenAtTheNextCommand,
			"where a fatal signal the shell sent itself is taken") {
		return r.status
	}
	return d.holding
}

// showTheExitTrap records shown as the status the EXIT trap reads for the
// death the shell has just taken — or, inside a function in the dialect
// where the death ends the call as killed, the status of a command killed by
// the signal. See Semantics.DeathInAFunctionEndsTheCallAsKilled.
//
// Nothing where the shell did not die: an orderly hangup leaves at its own
// status, and the trap reads that.
func (r *Runner) showTheExitTrap(shown int, name string, sig syscall.Signal) {
	if !r.diedOfItsOwnSignal() {
		return
	}
	if r.exitTrapRunsOnThisDeath(name, sig) && r.insideAFunctionCall() &&
		r.ask(r.sem().DeathInAFunctionEndsTheCallAsKilled,
			"the `$?` the EXIT trap reads after a death inside a function") {
		shown = r.signalDeathStatus(sig)
	}
	r.exitTrapShownStatus = &shown
}

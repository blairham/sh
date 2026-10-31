// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "syscall"

// The EXIT trap a fatal signal is running, and what happens inside it.
//
// A shell that runs its EXIT trap on a death — ExitTrapRunsOnSignalDeath — is
// dying while the trap runs, and the death is the one it ends by afterwards.
// What it is *not* while the trap runs is dead: the body is more of the
// script, and every command in it is a command. This shell held the death on
// the Runner throughout, which made the body a dead shell's in three ways,
// measured 2026-10-30 against bash 5.3.20 and ksh93u+ with `kill -TERM $$`
// and the line on the left as the trap:
//
//	kill $p; wait $p                 both stop the job; here kill sent nothing
//	kill -USR1 $$; echo after        bash dies of USR1 at once; here: after, TERM
//	(exit 5); echo "in $?"           in 5; here a `Signal 0` notice beside it
//
// The first is `trap 'kill $worker' EXIT`, which is the shape a script that
// cleans up after itself is written in, sending nothing: the death's note
// that the shell had ended itself told `kill` to stop before every target.
// The third is a copy of the shell made inside the trap taking the death with
// it and reporting itself killed. So the death is set aside while the body
// runs and put back afterwards — see Runner.setTheDeathAside — and the body
// runs as a living shell's.
//
// Then two questions the panel splits on, which the death had been hiding:
//
//   - An `exit` that names a status, in that trap. ksh93 exits at the status;
//     bash dies of the signal regardless. Semantics.
//     ExitInTheExitTrapOverridesASignalDeath, asked at the `exit` (#7063).
//   - A fatal signal while the trap runs. bash, zsh, dash and BusyBox ash die
//     of it at once; ksh93 ends the trap and leaves as it was leaving, at the
//     status or by the first signal. Semantics.
//     FatalSignalInTheExitTrapEndsOnlyTheTrap, asked where the signal would
//     have killed the shell (#7064). That one is not about a death at all —
//     `exit 3` and then a signal in the trap is 3 in ksh93 — but a death is
//     where it was found.
//
// And one signal neither question reaches, in the dialect that leaves it
// alone. ksh93 runs its EXIT trap on every fatal signal but SEGV: an
// untrapped SEGV kills it where it lands, with nothing written, from `kill
// -SEGV $$` and from another process alike — and in the EXIT trap it kills
// the shell where TERM would end only the trap. bash runs the trap for it as
// for the rest. Semantics.UntrappedSegvEndsTheShellWhereItLands, asked where
// the trap would run on the death and where the signal would end the trap
// (#7069).
//
// A signal from another process reaches the second question by the road a
// death from outside takes: the front end's watch hands it over, and the
// shell takes it at its next command. See outsidedeath.go, and
// Runner.publishTheTrapRunning for where the watch learns to.

// setAsideDeath is the death an EXIT trap is running ahead of.
type setAsideDeath struct {
	name    string
	sig     syscall.Signal
	stopped bool
}

// setTheDeathAside takes the death off the shell while its EXIT trap runs, so
// that the body is run as a living shell's: `kill` sends, a copy is not born
// dead, and a fatal signal in the body is a death of its own rather than one
// the shell already had. Runner.takeTheDeathBack puts it back.
func (r *Runner) setTheDeathAside() {
	r.exitTrapDeath = setAsideDeath{name: r.killedBy, sig: r.killedBySig, stopped: r.stoppedBySignal}
	r.killedBy, r.killedBySig, r.stoppedBySignal = "", 0, false
}

// takeTheDeathBack ends the shell's EXIT trap as far as the death is
// concerned: the one set aside is the one the shell dies of, unless the body
// died of a signal of its own — which is the second signal's death, in every
// dialect where that ends the shell — or an `exit` in it took the death away.
func (r *Runner) takeTheDeathBack() {
	d := r.exitTrapDeath
	r.exitTrapDeath = setAsideDeath{}
	r.stoppedBySignal = r.stoppedBySignal || d.stopped
	if r.killedBySig != 0 {
		return
	}
	r.killedBy, r.killedBySig = d.name, d.sig
}

// fatalSignalEndsOnlyTheExitTrap reports whether a fatal signal the shell
// would die of ends its EXIT trap instead, and ends the trap where it does.
//
// The shell at the top only: a copy's EXIT trap is a child's, and a signal
// aimed at `$$` from a copy is recorded for the shell above and reaches this
// as that shell's own. See Semantics.FatalSignalInTheExitTrapEndsOnlyTheTrap.
func (r *Runner) fatalSignalEndsOnlyTheExitTrap(name string) bool {
	if !r.inExitTrap || r.inSubshell ||
		!r.ask(r.sem().FatalSignalInTheExitTrapEndsOnlyTheTrap, "a fatal signal while the EXIT trap runs") {
		return false
	}
	if name == segvName && r.ask(r.sem().UntrappedSegvEndsTheShellWhereItLands,
		"an untrapped SEGV while the EXIT trap runs") {
		return false
	}
	r.exitTrapEndedBySignal = true
	r.stopTheShell()
	return true
}

// segvName is the one fatal signal a dialect may leave to kill the shell where
// it lands, ahead of everything a death would otherwise run first. See
// Semantics.UntrappedSegvEndsTheShellWhereItLands.
const segvName = "SEGV"

// segvEndsTheShellWhereItLands reports whether a fatal signal by this name is
// an untrapped SEGV in the dialect that lets one kill the shell where it lands:
// no EXIT trap is run for it, and one already running is not ended in place
// of the shell.
//
// Read and not asked, for the callers that decide where a death is held or
// handed over rather than whether a trap runs: an unanswered axis reads as
// no, so the death goes on to where the trap would run, and the axis is asked
// there — Runner.runExitTrap.
func (r *Runner) segvEndsTheShellWhereItLands(name string) bool {
	return name == segvName && r.sem().UntrappedSegvEndsTheShellWhereItLands == Yes
}

// exitOverridesTheDeath is an `exit` that named a status, in an EXIT trap the
// shell is running ahead of a death: in the dialect that says so the shell
// leaves at the status, and the death set aside is dropped. See
// Semantics.ExitInTheExitTrapOverridesASignalDeath.
//
// Asked only where there is a death to drop, which is only where the dialect
// runs an EXIT trap on one; a copy made inside the trap is the copy's `exit`.
func (r *Runner) exitOverridesTheDeath() {
	if !r.inExitTrap || r.inSubshell || r.exitTrapDeath.sig == 0 {
		return
	}
	if r.ask(r.sem().ExitInTheExitTrapOverridesASignalDeath, "an `exit` in the EXIT trap of a signal death") {
		r.exitTrapDeath = setAsideDeath{}
	}
}

// publishTheTrapRunning tells the front end's watch that the shell at the top
// is running its EXIT trap, in the dialect where a fatal signal ends only the
// trap, so that one from another process is handed over to do that rather
// than ending the process where it lands — and hands back what says the trap
// is done.
//
// Never at a prompt and never where no front end listens, as publishExitTrap.
// The answer is read and not asked, as the watch's answers are: an unanswered
// axis is no, and the signal ends the process as it did before.
func (r *Runner) publishTheTrapRunning() (done func()) {
	s := r.signals
	if s == nil || r.inSubshell || r.Interactive || !s.outside.heard.Load() ||
		r.sem().FatalSignalInTheExitTrapEndsOnlyTheTrap != Yes {
		return func() {}
	}
	s.outside.trapRunning.Store(true)
	if r.ListenForFatalSignals != nil {
		// TERM and INT come to the watch, which the runtime would
		// otherwise end the process with.
		r.ListenForFatalSignals()
	}
	return func() { s.outside.trapRunning.Store(false) }
}

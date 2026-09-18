// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "sync/atomic"

// A command substitution whose body will not parse ends the *script* in five
// of the seven columns, wherever it is written — and a subshell is not a
// boundary that contains it (#3274).
//
// # Why this is a box and not a boundary check
//
// The five columns read the body while they are reading the script's line, so
// when the failure happens there is no subshell yet to contain it. This engine
// reads the body at expansion time, deliberately — see the long note at the
// head of Runner.subst, where parsing it with the line was measured, costed
// and declined, because the body would have to be parsed with the alias table
// and the dialect as they stood at the line and the *tree* kept. So the
// outcome has to be produced without moving when the body is read, which is
// the same thing Diagnostics.BackquotedSubstitutionRestartsLines does one
// message over.
//
// A check at the subshell boundary would be seven checks: the parentheses, a
// pipeline element, a background job, a coprocess, a process substitution, a
// `$(<file)` read and the command substitution itself all clone. Seven copies
// of one rule is the shape this tree has been bitten by repeatedly — the
// eighth gets written by copying one of the seven and leaving the rule out.
// So the stop is recorded in a box every clone of a shell shares, exactly as
// a fatal signal a subshell aimed at the process is (see recordSharedDeath),
// and it is taken at the one sequence point every command already passes
// through.
//
// # What it is not
//
// It is not `exit`. `( exit 3 ); echo after` prints `after` in every column,
// and a subshell *is* a boundary for that — which is why this is a box of its
// own rather than a new abandonKind that controlExit would carry: the two
// unwind the same way and are caught in different places.
type scriptStop struct {
	stopped atomic.Bool
	// status is the dialect's syntax status, carried from the shell that
	// failed to the one that stops. Without it a failure inside a *pipeline
	// element* would end the script at the pipeline's own status: `v=$(echo
	// hi; for) | cat` is 2 in bash 5.3.20, 1 in zsh 5.9.2 and 2 in dash, and
	// `cat` succeeded in every one of them.
	status atomic.Int64
}

// recordScriptStop notes that a substitution's parse failure has ended the
// script, for the shell that is still running to stop at.
//
// The box is made by clone, before the copy, so a subshell and its parent
// share one — a box made lazily by whichever needed it first would be the
// subshell's own, and the parent would carry on. See Runner.clone.
func (r *Runner) recordScriptStop(status int) {
	if r.scriptStop == nil {
		// A runner with no clone below it has no box, and no reader either:
		// the stop it raised is already unwinding its own shell.
		return
	}
	r.scriptStop.status.Store(int64(status))
	r.scriptStop.stopped.Store(true)
}

// takeScriptStop reports such a stop, once.
//
// Once, because the shell stops on it here and everything after this point —
// an EXIT trap most of all — is still the shell's to run. Measured 2026-09-16
// on `trap 'printf "bye st=%s\n" "$?"' EXIT` with the failure inside `( … )`:
// bash 5.3.20 prints `bye st=2`, zsh 5.9.2 `bye st=1` and dash `bye st=2`, so
// the handler runs and sees the syntax status the script died of.
func (r *Runner) takeScriptStop() (status int, ok bool) {
	// A load before the swap, and it is not a micro-optimization: this runs
	// at every command of every script, and an unconditional swap would be a
	// write to a line a pipeline's elements share on each of them. The load
	// is the common case — nothing has failed — and the swap happens once in
	// the life of a shell that stops this way.
	if r.scriptStop == nil || !r.scriptStop.stopped.Load() {
		return 0, false
	}
	status = int(r.scriptStop.status.Load())
	// And the swap is still what takes it, because two shells may reach this
	// at once and only one of them may stop on it. Read before the swap for
	// the same reason: the shell that loses the race must not be the one that
	// cleared the status out from under the winner.
	if !r.scriptStop.stopped.Swap(false) {
		return 0, false
	}
	return status, true
}

// holdScriptStop hides a pending stop for the length of a trap body and hands
// back what puts it there again.
//
// The EXIT trap is the case it exists for, and it is the same treatment
// runExitTrap already gives r.ctl: the handler is more of the script and runs
// to its end, and the stop is still the shell's afterwards. Without this a
// subshell's own EXIT trap would consume the parent's stop — `( trap … EXIT;
// v=$(echo hi; for) )` would run its handler and then let the script carry on,
// which is neither column's answer.
func (r *Runner) holdScriptStop() func() {
	status, held := r.takeScriptStop()
	return func() {
		if held {
			r.recordScriptStop(status)
		}
	}
}

// substParseFailureStatus is the status a substitution body that would not
// parse leaves behind.
//
// The refusal's own number is the shell's syntax status, and one column
// reports a fatal error's instead wherever the failure is not the script's
// own line being read — see
// Semantics.SubstitutionParseFailureCarriesTheFatalStatus for the two panels
// and for the controls that say the 1 is neither the sourced-syntax status
// nor a failed redirection's.
//
// offTheScriptsLine is the caller saying so, and it is the gate as well as
// the question: the script's own line is the row every column agrees on, and
// asking there would move it. Two callers say yes — text `.` or `eval`
// borrowed, and a here-document body the shell carried on from — which are
// the same question asked at opposite ends.
func (r *Runner) substParseFailureStatus(offTheScriptsLine bool) int {
	if offTheScriptsLine &&
		r.ask(r.sem().SubstitutionParseFailureCarriesTheFatalStatus,
			"a substitution body that does not parse carrying a fatal error's status") {
		return r.fatalStatus()
	}
	return r.diag().SyntaxStatus()
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "syscall"

// A trapped signal a `( … )` or `$( … )` sends the shell, in the dialect that
// runs both in the shell's own process.
//
// The signal is aimed at `$$`, and in ksh93 `$$` is the process the subshell
// is running in. So the signal lands on the subshell: it ends there, as a
// command killed by the signal, and the shell's handler runs once it is over.
// Measured 2026-11-01 under `-c` on ksh93u+ 2012-08-01, five runs each and the
// same every time, `trap 'echo T' USR1` in front of each line:
//
//	(kill -USR1 $$; echo in); echo "after $?"        T, after 286
//	x=$(kill -USR1 $$; echo in); echo "after $?"     T, after 286, x empty
//	{ kill -USR1 $$; echo in; }; echo "after $?"     T, in, after 0
//	: | (kill -USR1 $$; echo in); echo "after $?"    T, after 286
//	(kill -USR1 $$; echo in) | cat; echo "after $?"  T, in, after 0
//
// bash 5.3.20, zsh 5.9.2 and dash write `in`, then `T`, then `after 0` for
// each subshell row: each runs the subshell in a child, so `$$` names the
// parent and the child runs on. The group row is the same everywhere, which
// is what keeps this off the top level. The last row is a pipeline element
// ksh93 forks, and it runs on there as well.
//
// The rest of the rule, measured the same day in ksh93:
//
//   - **Every unforked subshell around it ends too.** `( (kill -USR1 $$; echo
//     in1); echo in2 ); echo "after $?"` writes `T` and `after 286`, and a
//     `$( … )` inside parentheses or parentheses inside one are the same.
//   - **The handler runs in the shell, after the subshell.** `trap 'x=1' USR1`
//     leaves `x` set afterwards, and a name the subshell set is unset in the
//     handler.
//   - **Only the first signal counts.** The issue's `(kill -USR2 $$; echo
//     in1; kill -HUP $$; …)` writes `USR2` and `after 287`.
//   - **TERM and HUP the same.** `trap 'echo T' TERM; (kill $$; echo in)` is
//     `T` and `after 271`.
//
// Not modeled, and each recorded in docs/spec/semantics.md: the handler reads
// the status from before the `kill` there and the subshell's here; a trap the
// subshell set for the signal itself, and an ignore at the top, make ksh93
// end the parentheses at 261 with a `Trace/BPT trap` notice; and `kill -USR1
// $$ $$` runs the handler twice here. ksh93u+ has no `${.sh.pid}`, so the
// subshell has no number of its own to aim at.
//
// # The route
//
// sendSignal queues the handler as for any trapped signal aimed at `$$`, on
// the shell's list, and then ends the subshell at the killed status. Each
// unforked subshell around it ends as it ends, and the shell at the top runs
// the handler at its next command, as it runs any arrival.
//
// # Its EXIT trap, and the untrapped fatal half
//
// A subshell ended where its signal landed runs no EXIT trap of its own, and
// neither does any unforked subshell around it: the process that would run
// one is the shell, and only the shell's own trap runs, as the shell ends or
// at its own end. The same holds for an untrapped fatal signal, where
// Semantics.SubshellRunsOnAfterSignalingTheShell already stops the subshell
// in this dialect. Measured 2026-11-01 under `-c` on ksh93u+, each shell in
// a process group of its own, `trap 'echo bye' EXIT` in front of each row
// (#7153):
//
//	(trap 'echo sub' EXIT; kill -TERM $$; echo s2)    bye, killed by TERM
//	x=$(trap 'echo sub >&2' EXIT; kill -TERM $$; …)   bye, killed by TERM
//	: | (trap 'echo sub' EXIT; kill -TERM $$; …)      bye, killed by TERM
//	( (trap 'echo in' EXIT; kill -TERM $$); echo s2 ) bye, killed by TERM
//	(trap 'echo sub' EXIT; kill -SEGV $$; echo s2)    nothing, killed by SEGV
//	trap 'echo T' USR1, then the first row with USR1  T, after 286, bye
//
// with `trap 'echo outer' EXIT` on the outer parentheses of the fourth row and
// `echo "after $?"` after each. bash 5.3.20, zsh 5.9.2 and dash run every
// subshell's body and its trap to the end in every row: each ran the
// parentheses in a child the signal never reached. Here, before, ksh wrote
// `sub` (and `outer`) where ksh93 wrote nothing. The SEGV row is the first
// row with the shell's own trap skipped as well, which
// Semantics.UntrappedSegvEndsTheShellWhereItLands already answers.
//
// It is the signal aimed by the subshell's own `kill` and not any death
// during it: `/bin/kill -TERM $$` from inside the parentheses, the signal
// coming from another process while the shell waits for that child, writes
// `s2`, `sub` and `bye` in ksh93 as it does here.

// endsTheSubshellOnATrappedSignal ends r, a subshell the dialect runs in the
// shell's own process, after it has sent the shell a trapped signal, where
// the dialect says so. It reports whether r has been ended.
func (r *Runner) endsTheSubshellOnATrappedSignal(sig syscall.Signal) bool {
	if !r.subshellUnforked || !r.inTheShellsOwnProcess() ||
		!r.ask(r.sem().SubshellEndsWhenItSendsTheShellATrappedSignal,
			"whether a subshell ends when it sends the shell a trapped signal") {
		return false
	}
	r.status = r.signalDeathStatus(sig)
	r.stopTheShell()
	r.endedWhereItsSignalLanded = true
	return true
}

// endedWhereAFatalSignalLanded notes that r, a subshell the dialect has just
// stopped after it sent the shell an untrapped fatal signal, ended where the
// signal landed, when that was in the shell's own process. See
// Semantics.SubshellRunsOnAfterSignalingTheShell, which decided the stop.
func (r *Runner) endedWhereAFatalSignalLanded() {
	if r.subshellUnforked && r.inTheShellsOwnProcess() {
		r.endedWhereItsSignalLanded = true
	}
}

// takeOverAnEndWhereASignalLanded is the end of sub, a subshell r ran, which a
// signal it sent the shell ended where it landed: where r is an unforked
// subshell as well, it ends at the same status. It reports whether r has been
// ended.
func (r *Runner) takeOverAnEndWhereASignalLanded(sub *Runner) bool {
	if !sub.endedWhereItsSignalLanded || !r.subshellUnforked || !r.inTheShellsOwnProcess() {
		return false
	}
	r.status = sub.status
	r.stopTheShell()
	r.endedWhereItsSignalLanded = true
	return true
}

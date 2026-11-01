// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "syscall"

// A builtin writing past `ulimit -f`, and the SIGXFSZ that ends the shell
// where it stands.
//
// The kernel answers a write past the file-size limit as it answers a write
// into a pipe nobody reads: the write fails, and the writer is sent a signal
// whose default action ends it. A real shell's builtin is the shell, so the
// shell dies. Measured 2026-10-31, each shell in a process group of its own,
// `ulimit -f 1; printf '%4000s' x > f; echo after=$?`:
//
//	                       untrapped        trap '' XFSZ     trap 'h' XFSZ
//	bash 5.3, bash as sh   killed, silent   after=1          after=1, h ran
//	bash 3.2               killed, silent   after=1          after=1, h ran
//	ksh93, dash, zsh       killed, silent   after=1          after=1, h ran
//
// `echo "$x" > f` with a long $x is the same in every column. This shell
// reported the failed write in the dialect's wording and carried on at 1 —
// or at 0 in the zsh dialect, which keeps a builtin's own status for a
// closed descriptor — in all four dialects (#7057).
//
// So EFBIG asks about the disposition, exactly as EPIPE does in
// builtinWriteStatus: only the default action is a death, and an ignore or a
// handler leaves an ordinary failed write. A handler runs between commands,
// after the builtin's complaint, in the four shells that wait for one; zsh
// runs it at the write itself, inside the command's redirection.
//
// Answered here, at the write, rather than by the kernel's own copy, for the
// reason the driver gives in fatalsignal.go: the Go runtime catches SIGXFSZ
// and drops it while nothing is subscribed, and a write of a subshell, a
// pipeline element or a substitution body is this process's write, so a
// watch that died of the kernel's copy would end the whole shell for a
// subshell's overrun. The death is the Runner's own and reaches the process
// through DieBySignal, as a signal the shell sent itself does.
//
// With an EXIT trap the death runs it in the dialects that run one on a
// death, and the trap reads the status from before the builtin: `trap 'echo
// t $?' EXIT; (exit 5); printf … > f` writes `t 5` in bash 5.3, and nothing
// in dash and zsh, which die where they stand. bash 3.2 lets the builtin
// complain first, so its trap reads 1, and no dialect models it.
//
// ksh93 does not die there at all. Once an EXIT trap has been set it has
// caught the signal, and the write is a failed write the shell gives up
// over: `ksh: printf: write to 1 failed [File too large]`, then `t 5`, then
// exit 1, as over `echo $((1/0))` in the same place. That is
// Semantics.CaughtWritePastTheFileSizeLimitIsAFatalError, and the rows it
// does not reach — a POSIX-form function, a `( … )` — are recorded there.

// overrunEnding is how a builtin's write past `ulimit -f` ends: as any failed
// write does, in a death, or in a failed write the shell then gives up over.
type overrunEnding uint8

const (
	overrunFails overrunEnding = iota
	overrunDies
	overrunGivesUp
)

// answerAWritePastTheFileSizeLimit answers the SIGXFSZ a builtin's write past
// `ulimit -f` raised, and reports how the write ends.
//
// The kernel's copy is taken out of the runtime's hands on every road, the
// death included: a handler the shell above a subshell set is subscribed for
// it, and no panel shell runs that handler for a copy only the subshell had.
func (r *Runner) answerAWritePastTheFileSizeLimit() overrunEnding {
	arranged := r.signalArranged("XFSZ")
	r.writeSignalAbsorbed("XFSZ", arranged == signalHandledBy, 1)
	if arranged != signalFatal {
		return overrunFails
	}
	if r.caughtOverrunIsAFatalError() {
		return overrunGivesUp
	}
	shown := r.status
	r.signalDeath("XFSZ", syscall.SIGXFSZ)
	r.showTheExitTrap(shown, "XFSZ", syscall.SIGXFSZ)
	return overrunDies
}

// caughtOverrunIsAFatalError reports whether the shell has caught the
// SIGXFSZ an untrapped write past the limit raised, and gives up over the
// write rather than dying of it. Asked only of a shell that caught it: an
// EXIT trap set at some point, in a dialect that runs one on a death, at the
// shell's own level rather than in a subshell, and not at a prompt. See
// Semantics.CaughtWritePastTheFileSizeLimitIsAFatalError.
func (r *Runner) caughtOverrunIsAFatalError() bool {
	return r.caughtByAnExitTrap && !r.inSubshell && !r.Interactive &&
		r.sem().ExitTrapRunsOnSignalDeath == Yes &&
		r.ask(r.sem().CaughtWritePastTheFileSizeLimitIsAFatalError,
			"a builtin's write past the file-size limit once an EXIT trap has caught the signal")
}

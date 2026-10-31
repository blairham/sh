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
// t $?' EXIT; (exit 5); printf … > f` writes `t 5` in bash 5.3 and ksh93,
// and nothing in dash and zsh, which die where they stand. Two columns part
// from that and are not modeled: bash 3.2 lets the builtin complain first,
// so its trap reads 1, and ksh93 also lets it complain and then exits 1
// rather than dying of the signal.

// diedOfAWritePastTheFileSizeLimit answers the SIGXFSZ a builtin's write past
// `ulimit -f` raised, and reports whether the shell died of it.
//
// The kernel's copy is taken out of the runtime's hands on every road, the
// death included: a handler the shell above a subshell set is subscribed for
// it, and no panel shell runs that handler for a copy only the subshell had.
func (r *Runner) diedOfAWritePastTheFileSizeLimit() bool {
	arranged := r.signalArranged("XFSZ")
	r.writeSignalAbsorbed("XFSZ", arranged == signalHandledBy)
	if arranged != signalFatal {
		return false
	}
	shown := r.status
	r.signalDeath("XFSZ", syscall.SIGXFSZ)
	r.showTheExitTrap(shown, "XFSZ", syscall.SIGXFSZ)
	return true
}

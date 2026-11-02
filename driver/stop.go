// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import "syscall"

// stopThisProcess stops the shell with sig until something continues it, which
// is what `suspend` asks for and what `kill -TSTP $$` asks for.
//
// Here rather than in interp for the reason replaceProcess and dieBySignal are
// here: it is the *process* that stops, and a Runner embedded in another
// program must not be able to stop that program because a line of script said
// so. interp.Runner.StopThisProcess is nil until a binary that is a shell
// fills it in; `suspend` refuses while it is, and `kill` carries on.
//
// The signal is the caller's and this raises exactly that one, which is the
// whole of the hook. `suspend` hands it SIGSTOP, which nobody can decline. A
// `kill` aimed at the shell hands it the signal the script named, and the
// kernel then keeps every rule it has about that signal — the one that
// matters is that TSTP, TTIN and TTOU are discarded for an orphaned process
// group, measured in every reference, where a SIGSTOP in their place would
// hang a script nobody can continue. See interp.Runner.StopThisProcess.
//
// The group rather than the process would be the other reading, and it is not
// taken: this shell's process group holds whatever the shell has started, and
// a shell asking to stop *itself* has said nothing about them.
//
// It returns when a SIGCONT arrives, or at once where the kernel declined the
// stop, which is why `suspend`'s success is reported after this rather than
// before it. That promise is raiseStop's to keep, and on Linux it is not kept
// by a signal aimed at the process: see stop_linux.go.
func stopThisProcess(sig syscall.Signal) error {
	return raiseStop(sig)
}

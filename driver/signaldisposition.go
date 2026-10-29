// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"os/signal"
	"syscall"
)

// setSignalDisposition is interp.Runner.SetSignalDisposition for a shell that
// owns the process: `trap "" SIG` ignores the signal in the process, and
// `trap - SIG` hands it back to the default.
//
// The process-wide ignore is what a real shell's ignore is, and it is the only
// way a command this shell starts begins with the signal ignored: exec keeps
// SIG_IGN and resets everything handled. Measured 2026-10-29 with an ignored
// INT and then `/bin/sh -c 'kill -INT $$; echo survived'` — the child survives
// in bash 5.3, bash 3.2, ksh93, zsh and dash. A Runner embedded in another
// program cannot have that, because the same call takes every other
// subscription in the process away (#7044); interp keeps such a shell's
// ignore to itself, and this hook is what a binary that is the shell adds.
//
// signal.Reset for the other direction, which does not undo an ignore — interp
// takes one off by subscribing a drain afterwards, where the ignore was its own.
// See interp's dropOwnIgnore.
func setSignalDisposition(sig syscall.Signal, ignore bool) {
	if ignore {
		signal.Ignore(sig)
		return
	}
	signal.Reset(sig)
}

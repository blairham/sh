// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"os"
	"syscall"
)

// stopThisProcess stops the shell until something continues it, which is what
// `suspend` asks for.
//
// Here rather than in interp for the reason replaceProcess and dieBySignal are
// here: it is the *process* that stops, and a Runner embedded in another
// program must not be able to stop that program because a line of script said
// so. interp.Runner.StopThisProcess is nil until a binary that is a shell
// fills it in, and the builtin refuses while it is.
//
// SIGSTOP rather than SIGTSTP, and that is the one choice in this file worth
// stating. SIGTSTP is the keyboard's stop and can be caught, blocked or
// ignored — and it is ignored outright when the process group is orphaned,
// which is exactly the shape a shell run from a script or a test harness has.
// A `suspend` that returned 0 without stopping would be the wrong answer in
// the direction that matters, since the whole of the builtin is that it does
// not come back yet. SIGSTOP cannot be declined by anybody, so the stop either
// happens or the system call fails and says why.
//
// The group rather than the process would be the other reading, and it is not
// taken: this shell's process group holds whatever the shell has started, and
// a builtin asking to stop *the shell* has said nothing about them.
//
// It returns when a SIGCONT arrives, which is why the builtin's success is
// reported after this rather than before it.
func stopThisProcess() error {
	return syscall.Kill(os.Getpid(), syscall.SIGSTOP)
}

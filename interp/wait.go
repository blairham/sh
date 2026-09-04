// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"syscall"
	"time"
)

// How a command ended.
//
// A shell needs three answers where a library usually wants one: it exited,
// something killed it, or something *stopped* it and it is still there. The
// third is what ^Z is, and it is the one an ordinary wait cannot report —
// which is why a shell without job control does not hang on ^Z so much as
// wait forever for a process that is never going to finish.
type Wait struct {
	// Status is the exit status, when the command exited by itself.
	Status int

	// Signal is what killed or stopped it, when one did.
	Signal syscall.Signal

	// Killed and Stopped say which of the three happened. Both false means
	// it exited and Status is the answer.
	Killed  bool
	Stopped bool

	// User and System are the CPU the command spent, from the wait that
	// reaped it — wait4 hands them back beside the status, and a waiter
	// that reaps the child is the only place they can still be read. Zero
	// when the waiter did not ask, or when the command merely stopped and
	// has not been reaped at all. The `time` keyword's per-element report
	// is what reads them.
	User, System time.Duration
}

// waitResult turns a Wait into the status a script sees, and reports whether
// the command is still there.
//
// The encoding is the dialect's — 128 plus the signal in three of the four and
// 256 plus it in ksh93 — so it goes through the same axis a killed command
// already did rather than a second copy of the arithmetic.
func (r *Runner) waitResult(w Wait) (status int, stopped bool) {
	switch {
	case w.Stopped:
		return r.signalDeathStatus(w.Signal), true
	case w.Killed:
		return r.signalDeathStatus(w.Signal), false
	default:
		return w.Status, false
	}
}

// stoppedWait reports whether the command is still there.
func stoppedWait(w Wait) bool { return w.Stopped }

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package driver

import "syscall"

// readyExec is the exec's operands, held until the placement is done.
//
// Everywhere but Linux the execve goes through `syscall.Exec`, which converts
// the strings itself. So the window between the placement and the replacement
// still holds those conversions here, and the collector being stopped over it
// is still the whole of the mitigation — see replaceProcess.
//
// It is deliberate rather than unfinished, and there are two reasons.
//
// Go reaches execve through libc rather than through a raw system call on
// darwin, ios, openbsd, solaris, illumos and aix, and its own comment says a
// raw one should never be used there. Emptying the window means calling
// execve without allocating, and the call that would do it is not one this
// package is allowed to make. The platforms where it *would* be allowed —
// the other BSDs — are ones this change has no way to test on, and an
// untested raw execve is a worse bet than a window nobody has seen bite.
//
// And on the platform that matters most here it would buy nothing. macOS has
// its own instance of this bug (#799) and it is not a window at all: the
// runtime carries signals over a pipe, `exec 3>f` places the script's file
// over the pipe's descriptor, and the goroutine already blocked reading it
// throws immediately. There is no interval to empty — the descriptor is gone
// from the moment it is placed — so nothing about when the conversions run
// changes the outcome. Measured: a forced allocation in this window has never
// failed on macOS at any size, and #799 fails there every time with no forced
// anything.
type readyExec struct {
	path string
	argv []string
	env  []string
}

func prepareExec(path string, argv, env []string) (readyExec, error) {
	return readyExec{path: path, argv: argv, env: env}, nil
}

// undo has nothing to put back: syscall.Exec does its own open-file limit
// restore and does it after this point.
func (readyExec) undo() {}

func (r readyExec) execve() error {
	reachedPlacement("execve")
	return syscall.Exec(r.path, r.argv, r.env)
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package driver

import (
	"os"
	"syscall"
)

// raiseStop raises sig on the whole process. On macOS that is enough: the
// kernel stops a process in the call that sends it the stop, so the shell is
// stopped before this returns, which is what stop_linux.go has to arrange by
// hand. Measured by TestAStopAimedAtTheShellStopsTheProcessWithIt, which finds
// nothing written before the stop in 20 of 20 runs with -race and without.
func raiseStop(sig syscall.Signal) error {
	return syscall.Kill(os.Getpid(), sig)
}

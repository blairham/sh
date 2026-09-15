// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package testenv

import (
	"io/fs"
	"os"
	"syscall"
)

// WriteExecutable writes a file a test is about to run, and is a drop-in for
// os.WriteFile everywhere a suite makes a fixture it then executes.
//
// # Why the plain write is not enough
//
// Linux refuses to exec a file that any process still holds open for writing:
// ETXTBSY, `text file busy`. `os.WriteFile` closes its own descriptor before
// it returns, so a test is never racing itself — the holder is somebody else's
// *child*. This is golang/go#22315. Go opens files O_CLOEXEC, so a process
// forked anywhere else in the same program loses the descriptor at its own
// exec — but it holds it, open for writing, for the whole window between its
// fork and that exec. A test suite full of `t.Parallel` tests that start
// external commands has that window open more or less continuously, and an
// exec of a freshly written fixture inside it fails.
//
// The failure is a flake with a plausible wrong story attached: the shell
// reports `text file busy` and exits 126, so the test reads as the shell
// having refused a file it should have run (#2787, and #1623 in
// internal/startupcost before it).
//
// # Why holding ForkLock is the whole of the fix
//
// syscall.ForkLock is held for writing across a forkExec, and on Linux
// os/exec's child is started with CLONE_VFORK — so the parent, and with it the
// lock, is suspended until the child has reached its own exec and dropped
// every close-on-exec descriptor it inherited. While this holds the lock there
// is therefore no child anywhere in the process sitting between a fork and an
// exec, and none can start. The window is not narrowed; it is closed.
//
// Measured, in a golang container on Linux, with eight goroutines starting
// `/bin/sh -c :` in a loop and 400 rounds of write-then-exec alongside them:
// 34 of the 400 execs failed with ETXTBSY with the plain write, and 0 of 400
// failed with the write under this lock. TestAFreshlyWrittenExecutableRuns is
// that measurement, kept.
//
// macOS was measured not to enforce the rule at all — a file with a writer
// still open on it executes there without complaint — which is why only the
// ubuntu job has ever seen this. The lock is taken on every platform anyway:
// it costs a mutex against a file write, and a guard that is only taken where
// the failure has already been seen is a guard the next platform will not have.
//
// It lives in this package for the reason the package itself exists: a helper
// a suite has to copy to reach a package is a helper the next package will not
// have, and the copy is where the fix goes missing.
func WriteExecutable(path string, content []byte, mode fs.FileMode) error {
	syscall.ForkLock.Lock()
	defer syscall.ForkLock.Unlock()
	return os.WriteFile(path, content, mode)
}

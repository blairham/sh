// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package zsh

import (
	"os"
	"time"
)

// A platform with no record locking, and `zsystem supports flock` says so.
//
// The builtin is still registered, which is the difference between this and
// the shape systemio_other.go takes for `sysopen`: `zsystem` is more than its
// lock — `zsystem supports` is the question a script asks *before* it commits
// to one — and answering that question truthfully is exactly what a caller
// needs from a shell that cannot lock. See zsystemLock, which refuses.

const systemLockSupported = false

func systemLockOpen(string, bool) (*os.File, error) { panic("unreachable: flock is unsupported here") }

func systemLockTake(int, bool, bool, time.Duration, time.Duration) (bool, error) {
	panic("unreachable: flock is unsupported here")
}

func systemLockRelease(int) {}

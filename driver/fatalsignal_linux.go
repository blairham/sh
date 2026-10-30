// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"os"
	"syscall"
)

// platformThrowingSignals are this kernel's signals beyond the shared list
// that the runtime turns into a goroutine dump: STKFLT, whose default action
// ends the process here.
var platformThrowingSignals = []os.Signal{syscall.SIGSTKFLT}

// platformDiscardedSignals are this kernel's signals beyond the shared list
// whose default action ends the process and which the runtime catches and
// drops: IO and PWR, and the real-time numbers the runtime offers to
// os/signal.
//
// What each of them does to a shell is interp's measurement and not this
// file's — see interp/platformsignals_linux.go, where IO ends the shell here
// (where on a BSD it is discarded) and every number from 32 to 64 ends it in
// all five columns. 32 to 34 are not listed because os/signal cannot be asked
// for them: the runtime keeps those three for its own threading.
var platformDiscardedSignals = func() []os.Signal {
	sigs := []os.Signal{syscall.SIGIO, syscall.SIGPWR}
	for n := firstRealTimeTheRuntimeHandles; n <= lastRealTimeSignal; n++ {
		sigs = append(sigs, syscall.Signal(n))
	}
	return sigs
}()

// The real-time range as this runtime sees it on this kernel: it keeps 32
// through 34 for itself and handles 35 onward like any other signal nothing
// has asked for.
const (
	firstRealTimeTheRuntimeHandles = 35
	lastRealTimeSignal             = 64
)

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import "syscall"

// platformEndsTheShellFromOutside is what this kernel adds to the signals
// whose default action ends a shell sent one from another process. Each one's
// default here is interp's measurement — see interp/platformsignals_linux.go —
// and a real-time number stands for the range the runtime offers os/signal.
var platformEndsTheShellFromOutside = []namedSignal{
	{"STKFLT", syscall.SIGSTKFLT},
	{"IO", syscall.SIGIO},
	{"PWR", syscall.SIGPWR},
	{"40", syscall.Signal(40)},
}

// trappedProfFromOutside is what `trap 'echo caught' PROF; sleep 1; echo after`
// writes when a PROF arrives from another process. The handler never runs on
// this kernel, where every reference runs it: nothing in a Go program can hear
// a PROF the runtime keeps for its profiler, and the one way around it, a
// handler written outside Go, was decided against (2026-11-04, #7056 stays
// open for this cell). The shell surviving it is what this row asserts — the
// trap put the runtime's handler back over the default.
const trappedProfFromOutside = "after\n"

// standardStreamCopyArrivesApart is whether the runtime's own SIGPIPE for a
// broken standard output reaches the shell apart from the kernel's. Not
// reliably here: both are forwarded, and they usually fold into one arrival.
const standardStreamCopyArrivesApart = false

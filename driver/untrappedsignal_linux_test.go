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

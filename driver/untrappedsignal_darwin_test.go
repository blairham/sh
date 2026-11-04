// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import "syscall"

// platformEndsTheShellFromOutside is what this kernel adds to the signals
// whose default action ends a shell sent one from another process: EMT,
// measured 2026-10-29 at 135 in bash 5.3, bash 3.2, ksh93, zsh and dash.
var platformEndsTheShellFromOutside = []namedSignal{{"EMT", syscall.SIGEMT}}

// trappedProfFromOutside is what `trap 'echo caught' PROF; sleep 1; echo after`
// writes when a PROF arrives from another process: the handler runs, as it
// does in every reference, because the kqueue hears what the runtime keeps
// for its profiler (#7056).
const trappedProfFromOutside = "caught\nafter\n"

// standardStreamCopyArrivesApart is whether the runtime's own SIGPIPE for a
// broken standard output reaches the shell apart from the kernel's: here
// always, because the runtime drops the kernel's and the kqueue passes it on.
const standardStreamCopyArrivesApart = true

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import "syscall"

// platformEndsTheShellFromOutside is what this kernel adds to the signals
// whose default action ends a shell sent one from another process: EMT,
// measured 2026-10-29 at 135 in bash 5.3, bash 3.2, ksh93, zsh and dash.
var platformEndsTheShellFromOutside = []namedSignal{{"EMT", syscall.SIGEMT}}

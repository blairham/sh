// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"os"
	"syscall"
)

// platformThrowingSignals are this kernel's signals beyond the shared list
// that the runtime turns into a goroutine dump.
//
// EMT is the one. Its default action ends the process, and the runtime marks
// it as throwing without offering it to os/signal by default, so one sent
// from another process printed `SIGEMT: emulate instruction executed` and a
// page of goroutines, and exited 2. Measured 2026-10-29 with `/bin/kill -EMT`
// from another process at a shell parked in a builtin `read`, an external
// `sleep` and a busy loop: bash 5.3, bash 3.2, ksh93, zsh and dash each die of
// it in all three, printing nothing.
var platformThrowingSignals = []os.Signal{syscall.SIGEMT}

// platformDiscardedSignals are this kernel's signals beyond the shared list
// whose default action ends the process and which the runtime catches and
// drops. There are none here: IO and INFO are dropped by the runtime, and
// dropping them is their default action on this kernel too.
var platformDiscardedSignals []os.Signal

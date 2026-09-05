// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"syscall"
	"unsafe"
)

// dispositionBytes is a buffer comfortably larger than this kernel's struct
// sigaction, zeroed. die_darwin.go carries the reasoning; it holds here for
// the same reason and with more force, because this structure's layout
// differs between architectures where the zeroed buffer does not.
const dispositionBytes = 64

// sigsetBytes is the size of a sigset_t this kernel is told it was handed. It
// is 8 on every Linux architecture — 64 signals, one bit each — and the call
// fails with EINVAL rather than guessing if it disagrees.
const sigsetBytes = 8

// restoreDefaultDisposition puts a signal's handling back to the kernel's
// default action, undoing whatever the Go runtime installed at startup.
//
// Asked of the kernel directly because nothing in os/signal can say it: see
// dieBySignal for what the runtime does with a signal nothing is listening
// for, and why "nothing" is not the same as "the default".
//
// No restorer is supplied and none is needed: a restorer is the address the
// kernel returns a signal *handler* through, and the default action never
// enters this process at all.
func restoreDefaultDisposition(sig syscall.Signal) error {
	var act [dispositionBytes]byte
	if _, _, errno := syscall.Syscall6(
		syscall.SYS_RT_SIGACTION, uintptr(sig), uintptr(unsafe.Pointer(&act)), 0,
		sigsetBytes, 0, 0,
	); errno != 0 {
		return errno
	}
	return nil
}

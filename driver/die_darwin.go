// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"syscall"
	"unsafe"
)

// dispositionBytes is a buffer comfortably larger than this kernel's struct
// sigaction, zeroed.
//
// The size is deliberately generous and the *layout* is deliberately not
// written down. Every field of that structure has to be zero for what this
// asks — the default handler, no trampoline, an empty mask, no flags — and an
// all-zero buffer is that structure on every architecture without this file
// having to agree with the header about where each field sits. Getting the
// offsets wrong would be a silent misconfiguration of a signal; getting the
// size wrong in this direction cannot be, because the kernel reads as much of
// it as its own structure is and no more.
const dispositionBytes = 64

// restoreDefaultDisposition puts a signal's handling back to the kernel's
// default action, undoing whatever the Go runtime installed at startup.
//
// Asked of the kernel directly because nothing in os/signal can say it: see
// dieBySignal for what the runtime does with a signal nothing is listening
// for, and why "nothing" is not the same as "the default".
func restoreDefaultDisposition(sig syscall.Signal) error {
	var act [dispositionBytes]byte
	if _, _, errno := syscall.Syscall(
		syscall.SYS_SIGACTION, uintptr(sig), uintptr(unsafe.Pointer(&act)), 0,
	); errno != 0 {
		return errno
	}
	return nil
}

// ignoredByTheKernel reports whether sig's disposition is SIG_IGN, asked of
// the kernel for the reason restoreDefaultDisposition sets it there: what
// os/signal reports is its own bookkeeping, and after a subscription has come
// and gone the two can disagree. See letGoOfTheWatch.
func ignoredByTheKernel(sig syscall.Signal) bool {
	var old [dispositionBytes]byte
	if _, _, errno := syscall.Syscall(
		syscall.SYS_SIGACTION, uintptr(sig), 0, uintptr(unsafe.Pointer(&old)),
	); errno != 0 {
		return false
	}
	// The handler is the structure's first word; SIG_IGN is 1.
	return *(*uintptr)(unsafe.Pointer(&old)) == 1
}

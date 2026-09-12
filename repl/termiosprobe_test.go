// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package repl

import (
	"syscall"
	"unsafe"
)

// An independent reader of a terminal's line discipline, for the tests that
// check what raw mode did.
//
// Deliberately **not** internal/tty, which is what the editor uses to set the
// mode. A test that read the terminal back through the same code that wrote it
// would pass whenever the two halves agreed, including when both were wrong
// about which flag they were touching — the same reason internal/acpcheck
// writes out the wire format again by hand rather than importing the package
// it grades. Two ioctls is a cheap price for an instrument that can disagree
// with the thing it measures.
func probeTermios(fd int) (syscall.Termios, error) {
	var t syscall.Termios
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), probeTcGets,
		uintptr(unsafe.Pointer(&t)), 0, 0, 0)
	if errno != 0 {
		return t, errno
	}
	return t, nil
}

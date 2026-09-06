// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package tty

import (
	"syscall"
	"unsafe"
)

// isTerminalFd asks the kernel for the descriptor's terminal attributes.
//
// Through syscall directly, for the reason the rest of this module's terminal
// code is: there is no dependency here outside the linter's tooling, and a
// shell reaching for a terminal library to ask one ioctl would be a poor trade.
//
// The call is the same one raw mode begins with, which is not a coincidence:
// "can I read this thing's line discipline" *is* the question, and a thing that
// has one is a terminal.
func isTerminalFd(fd uintptr) bool {
	var t syscall.Termios
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, fd, tcGets,
		uintptr(unsafe.Pointer(&t)), 0, 0, 0)
	return errno == 0
}

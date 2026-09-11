// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package tty

import (
	"syscall"
	"unsafe"
)

// winsize is what TIOCGWINSZ fills in. The kernel reports a terminal in both
// characters and pixels; the pixel pair is never wanted here, and has to be
// there for the ioctl to land in the right fields.
type winsize struct {
	rows, cols     uint16
	xpixel, ypixel uint16
}

// sizeOfFd asks the kernel how big the terminal behind this descriptor is.
//
// One ioctl answers both numbers, and it is *this* one: a helper that asked
// for rows on its own would be a second reader of a question that already has
// one.
func sizeOfFd(fd uintptr) (rows, cols int) {
	var ws winsize
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, fd, syscall.TIOCGWINSZ,
		uintptr(unsafe.Pointer(&ws)), 0, 0, 0)
	if errno != 0 {
		return 0, 0
	}
	return int(ws.rows), int(ws.cols)
}

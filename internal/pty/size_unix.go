// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build darwin || linux

package pty

import (
	"os"
	"syscall"
	"unsafe"
)

// winsize is what TIOCSWINSZ reads. The kernel keeps a terminal's size in
// both characters and pixels; only the characters are set here, and the rest
// of the struct has to be there for the ioctl to land in the right fields.
type winsize struct {
	rows, cols     uint16
	xpixel, ypixel uint16
}

// SetSize gives a pseudo-terminal a size.
//
// A freshly opened pair is zero by zero, and a line editor asking how wide the
// terminal is gets 0 — which is a real answer meaning "do not know" and is not
// the answer a session has. A shell driven through a pty with no size set is
// therefore being asked to draw for a terminal nobody has, so the wrapping and
// redraw a test is watching are not the ones a person sees.
//
// Set on the terminal end, which is the side the shell holds.
func SetSize(terminal *os.File, rows, cols int) error {
	ws := winsize{rows: uint16(rows), cols: uint16(cols)}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, terminal.Fd(),
		syscall.TIOCSWINSZ, uintptr(unsafe.Pointer(&ws)))
	if errno != 0 {
		return errno
	}
	return nil
}

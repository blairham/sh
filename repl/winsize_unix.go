// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package repl

import (
	"os"
	"syscall"
	"unsafe"
)

// winsize is what TIOCGWINSZ fills in. The kernel reports the terminal in
// both characters and pixels; the pixel pair is never wanted here, and has to
// be there for the ioctl to land in the right fields.
type winsize struct {
	rows, cols     uint16
	xpixel, ypixel uint16
}

// terminalSize is how many rows and columns the terminal has, or zeroes where
// it will not say.
//
// Asked fresh every time it is needed rather than cached and kept current
// with SIGWINCH. A window that changed size between two keystrokes is the
// normal case, not the exception — the user drags the corner while the shell
// sits at a prompt — and a cached size is wrong for exactly as long as it
// takes the next signal to arrive. This is one ioctl on a path that is
// already waiting for a person to press a key.
//
// Zero is a real answer and means "do not know": a pipe, a closed terminal,
// or a kernel that declines. Every caller has to have something sensible to
// do with it, because the editor still has to draw and $COLUMNS must not be
// assigned a lie.
//
// One ioctl answers both numbers, and it is *this* one: a second helper that
// asked the kernel for rows on its own would be the fifth time in this
// repository that a duplicated question got fixed in one copy. terminalWidth
// is a name for the column half of this and not a second reader.
func terminalSize(f *os.File) (rows, cols int) {
	if f == nil {
		return 0, 0
	}
	var ws winsize
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, f.Fd(),
		syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&ws)), 0, 0, 0)
	if errno != 0 {
		return 0, 0
	}
	return int(ws.rows), int(ws.cols)
}

// terminalWidth is the column half, for the editor, which draws in columns and
// has nothing to do with rows.
func terminalWidth(f *os.File) int {
	_, cols := terminalSize(f)
	return cols
}

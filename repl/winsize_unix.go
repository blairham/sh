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
// both characters and pixels; only the column count is wanted here, and the
// rest of the struct has to be there for the ioctl to land in the right
// fields.
type winsize struct {
	rows, cols     uint16
	xpixel, ypixel uint16
}

// terminalWidth is how many columns the terminal has, or 0 if it will not
// say.
//
// Asked fresh every time it is needed rather than cached and kept current
// with SIGWINCH. A window that changed size between two keystrokes is the
// normal case, not the exception — the user drags the corner while the shell
// sits at a prompt — and a cached width is wrong for exactly as long as it
// takes the next signal to arrive. This is one ioctl on a path that is
// already waiting for a person to press a key.
//
// Zero is a real answer and means "do not know": a pipe, a closed terminal,
// or a kernel that declines. Every caller has to have something sensible to
// do with it, because the editor still has to draw.
func terminalWidth(f *os.File) int {
	if f == nil {
		return 0
	}
	var ws winsize
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, f.Fd(),
		syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&ws)), 0, 0, 0)
	if errno != 0 {
		return 0
	}
	return int(ws.cols)
}

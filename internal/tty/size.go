// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package tty

import "os"

// Size is how many rows and columns a terminal has, or zeroes where it will
// not say.
//
// The second question this package answers, and it is here for the reason
// IsTerminal is: two packages have to ask it and the dependency runs one way.
// `repl` has had the ioctl since the line editor needed a width; `interp`
// cannot reach `repl`, and a shell whose `$COLUMNS` is a parameter of the
// language has to ask the same question from the other side of that edge. A
// second reader would be the sixth time in this repository that a duplicated
// question got fixed in one copy, so `repl` reads this one too — see
// repl.terminalSize, which is now a name for this call and not a second
// implementation of it.
//
// Asked fresh every time rather than cached and kept current with SIGWINCH. A
// window that changed size between two reads is the normal case, not the
// exception, and a cached size is wrong for exactly as long as it takes the
// next signal to arrive. It is one ioctl.
//
// Zero is a real answer and means "do not know": a pipe, a closed terminal, or
// a kernel that declines. Every caller has to have something sensible to do
// with it, because the editor still has to draw and the shells that keep the
// pair as parameters disagree about what an unanswerable terminal is worth —
// measured, zsh 5.9.2 reports `0` and bash leaves both names unset.
//
// The descriptor is borrowed through SyscallConn rather than taken with Fd,
// for the lifetime reason IsTerminal gives: `Control` holds a reference for
// the length of the call, so a `Close` racing this cannot free the number and
// let the ioctl land on whatever the kernel handed out next.
func Size(f *os.File) (rows, cols int) {
	if f == nil {
		return 0, 0
	}
	rc, err := f.SyscallConn()
	if err != nil {
		return 0, 0
	}
	if err := rc.Control(func(fd uintptr) { rows, cols = sizeOfFd(fd) }); err != nil {
		return 0, 0
	}
	return rows, cols
}

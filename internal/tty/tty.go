// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package tty answers one question: is this open file a terminal.
//
// It exists because two packages have to ask it and only one of them could.
// `repl` has had the exact answer since #509 — an ioctl, next to the termios
// code that needs the same call — and `interp` could not reach it, because
// `repl` imports `interp` and the dependency runs one way. So `interp` had an
// approximation of its own, and #525 is what the approximation cost.
//
// # Why the question has exactly one right answer
//
// A character device is a strictly weaker test, and the difference is not
// theoretical. `/dev/null` is a character device, and it is what cron, systemd,
// CI and every test harness hand a shell for standard input; `/dev/zero`,
// `/dev/random`, a serial port and a printer are the same shape. A shell that
// reads any of them as a terminal prompts into something nobody is looking at.
//
// Measured 2026-09-06, `read -p 'PROMPT-42 ' v < DEVICE`, bash 5.3.15, bash
// 3.2.57 and bash-as-sh, all three identical:
//
//	/dev/null     no prompt, status 1
//	/dev/random   no prompt, status 0 — it read a line and never asked for one
//	a terminal    PROMPT-42 printed
//
// `/dev/random` is the case that says this is about terminals and not about
// the null device: the read *succeeds* there, so a shell has every reason to
// have printed a prompt, and bash does not. The same measurement against this
// shell before this change printed `PROMPT-42 ` — and `select` under the ksh93
// dialect drew its `#? ` where real ksh93 draws none.
//
// Asking the kernel for the terminal attributes gets ENOTTY from every one of
// those devices and a filled-in struct from a real terminal, without being
// told about any of them by name. So the special case for the null device that
// `interp` used to carry is not moved here — it is gone, which is the check
// #525 asked for.
//
// # Why interp may ask this
//
// It is a question about a descriptor the Runner was *handed*, not about the
// process it is running in — the same kind of question `Stat` is, only answered
// correctly. A Runner embedded in another program may have been given a pipe
// while the program around it sits at a terminal, and this still answers about
// the shell's input. Nothing here reads the environment, opens a path, or asks
// where the process is.
package tty

import "os"

// IsTerminal reports whether this open file is a terminal.
//
// The descriptor is borrowed through SyscallConn rather than taken with Fd,
// and the reason is not the one this code inherited. **The old reason has
// expired**: `os.File.Fd` used to detach the file from the runtime's poller
// and leave it in blocking mode for good, so asking a pipe a question it was
// going to answer no to cost it its deadlines. Measured on the pinned
// toolchain, go1.26.1, that is no longer true — a pipe still takes a
// `SetReadDeadline` and still times out after `Fd` has been called on it. The
// comment saying otherwise was written against an older Go and was carried
// here from `repl` unexamined; a mutation run is what asked.
//
// The reason that has not expired is lifetime. `Control` holds a reference for
// the length of the call, so a `Close` racing this cannot free the descriptor
// number and let the ioctl land on whatever the kernel handed out next. `Fd`
// gives a number and no such promise. That is a smaller claim than the old one
// and it is the true one, so it is the one written down.
func IsTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	rc, err := f.SyscallConn()
	if err != nil {
		return false
	}
	isTTY := false
	if err := rc.Control(func(fd uintptr) { isTTY = isTerminalFd(fd) }); err != nil {
		return false
	}
	return isTTY
}

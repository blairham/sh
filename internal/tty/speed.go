// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package tty

import "os"

// OutputSpeed is the terminal's output speed in bits per second, or 0 where f
// is not a terminal or the speed cannot be read.
//
// A shell that pads its control sequences reads it once, as zsh does when it
// sets its terminal up: a description's `$<n>` delay is that many
// milliseconds of NUL bytes at this speed. See dialect/zsh's padded.
func OutputSpeed(f *os.File) int {
	if f == nil || !IsTerminal(f) {
		return 0
	}
	rc, err := f.SyscallConn()
	if err != nil {
		return 0
	}
	speed := 0
	if err := rc.Control(func(fd uintptr) { speed = outputSpeedFd(fd) }); err != nil {
		return 0
	}
	return speed
}

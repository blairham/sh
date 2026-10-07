// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package tty

import "syscall"

// linuxSpeeds turns Linux's speed codes into the speeds they stand for.
var linuxSpeeds = map[uint32]int{
	syscall.B50: 50, syscall.B75: 75, syscall.B110: 110, syscall.B134: 134,
	syscall.B150: 150, syscall.B200: 200, syscall.B300: 300, syscall.B600: 600,
	syscall.B1200: 1200, syscall.B1800: 1800, syscall.B2400: 2400,
	syscall.B4800: 4800, syscall.B9600: 9600, syscall.B19200: 19200,
	syscall.B38400: 38400, syscall.B57600: 57600, syscall.B115200: 115200,
	syscall.B230400: 230400,
}

// cbaud is the mask of the speed code in the control flags, CBAUD in
// termios.h, which package syscall does not name.
const cbaud = 0o10017

// outputSpeedFd reads the speed, which Linux stores as a code in the
// control flags.
func outputSpeedFd(fd uintptr) int {
	var t syscall.Termios
	if err := ioctl(fd, tcGets, &t); err != nil {
		return 0
	}
	return linuxSpeeds[t.Cflag&cbaud]
}

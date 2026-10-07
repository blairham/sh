// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package tty

import "syscall"

// outputSpeedFd reads the speed, which the BSDs store as the number itself.
func outputSpeedFd(fd uintptr) int {
	var t syscall.Termios
	if err := ioctl(fd, tcGets, &t); err != nil {
		return 0
	}
	return int(t.Ospeed)
}

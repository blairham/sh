// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package fdset

import (
	"syscall"
	"unsafe"
)

// Pending is how many bytes a read on fd would return at once: what the
// terminal has already delivered and nobody has read. zsh hands the same
// count to a widget as `$PENDING`, and highlighters and suggesters skip their
// work for a key with more input behind it (#6363).
//
// Where the count cannot be asked, the answer is ReadableNow's as a count —
// one byte or none — which is right for every caller that asks whether there
// is more rather than how much.
func Pending(fd int) int {
	if fd < 0 {
		return 0
	}
	var n int32
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), fionread, uintptr(unsafe.Pointer(&n)))
	if errno != 0 || n < 0 {
		if ReadableNow(fd) {
			return 1
		}
		return 0
	}
	return int(n)
}

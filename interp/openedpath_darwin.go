// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"syscall"
	"unsafe"
)

// fGetPath is Darwin's F_GETPATH: fill a buffer with the path of the file the
// descriptor holds. Named here rather than taken from a header because the
// standard library does not export it, and it is a stable part of the
// kernel's fcntl interface.
const fGetPath = 50

// maxPathBytes is MAXPATHLEN, the size F_GETPATH documents its buffer must
// be. The kernel writes a NUL-terminated string of at most this length, so a
// buffer of exactly this size cannot be overrun and a smaller one is not
// allowed.
const maxPathBytes = 1024

// pathOfFd asks the kernel for the path of an open descriptor.
//
// Measured on Darwin 25.5.0, and the measurements are why the callers can be
// simple: a file opened through a symbolic link answers with the target's
// path, a path containing `..` answers with the walked-through form, and a
// path under `/tmp`, `/var` or `/etc` answers under `/private` — the same
// aliases the policy already normalizes when it reads a rule, which is what
// keeps this from denying every ordinary access on a Mac. A pipe answers
// EBADF, which is the nameless case.
func pathOfFd(fd uintptr) (string, bool) {
	buf := make([]byte, maxPathBytes)
	if _, _, errno := syscall.Syscall(
		syscall.SYS_FCNTL, fd, fGetPath, uintptr(unsafe.Pointer(&buf[0])),
	); errno != 0 {
		return "", false
	}
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i]), i > 0
		}
	}
	// A kernel that filled the buffer without terminating it has not answered
	// the question, and a truncated path is worse than no path: it would name
	// a *different* place, which a policy could match.
	return "", false
}

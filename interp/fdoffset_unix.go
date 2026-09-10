// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp

import (
	"io"
	"syscall"
)

// seekCurrent asks the kernel where an open descriptor's file position is,
// without moving it.
//
// A seek of zero from the current position, which is the one call that reports
// a position rather than setting one. Not through an *os.File wrapper around
// the number: os.NewFile attaches a finalizer that closes the descriptor, and
// the descriptor here belongs to the shell's own table and must outlive the
// question.
func seekCurrent(fd int) (int64, bool) {
	off, err := syscall.Seek(fd, 0, io.SeekCurrent)
	if err != nil {
		return 0, false
	}
	return off, true
}

// seekTo moves an open descriptor's file position, and reports whether the
// kernel took the request.
//
// The writing half of seekCurrent and reached the same way and for the same
// reason: through the number, never through an *os.File wrapper built around
// it, because such a wrapper's finalizer would close a descriptor the shell's
// table still owns.
//
// A position before the start of the file is refused by the kernel rather than
// clamped here, and a descriptor with no position at all — a pipe, a terminal
// — is refused as well. Both come back as the same false, which is what the
// caller's question had as its answer either way.
func seekTo(fd int, offset int64, whence int) bool {
	_, err := syscall.Seek(fd, offset, whence)
	return err == nil
}

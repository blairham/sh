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

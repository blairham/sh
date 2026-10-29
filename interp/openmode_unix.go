// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp

import "syscall"

// descriptorOpenForWriting reports whether fd was opened for writing as well
// as for reading — `exec 3<>fifo` rather than `exec 3<fifo`, or a pipe's read
// end, which is read-only.
//
// It is the kernel's answer about the open, not about the object: a fifo
// opened both ways and the same fifo opened to be read are the same file and
// two different descriptors. ksh93 gives a `read` up differently on the two
// (#7027); see ReadAbandonedOnlyAtTheStartOfALine. A descriptor the question
// cannot be put to is answered as read-only.
func descriptorOpenForWriting(fd int) bool {
	flags, err := fcntlInt(fd, syscall.F_GETFL, 0)
	return err == nil && flags&syscall.O_ACCMODE == syscall.O_RDWR
}

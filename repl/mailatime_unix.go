// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package repl

import (
	"io/fs"
	"syscall"
	"time"
)

// accessTime is when a file was last read, which is the half of the mail
// question the portable interface does not carry.
//
// [io/fs.FileInfo] gives the modification time and no more, so this reaches
// for the platform's own record. The second result is false where it cannot
// be had, and a mailbox whose access time is unknown is reported about by
// nobody — silence being the answer that cannot be wrong for a shell that has
// no way to tell read from unread.
func accessTime(info fs.FileInfo) (time.Time, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, false
	}
	sec, nsec := atimeParts(st)
	return time.Unix(sec, nsec), true
}

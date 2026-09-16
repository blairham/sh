// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"io/fs"
	"syscall"
	"time"
)

// fileChangeTime is when the file's inode last changed, which one dialect's
// sort parameter can be asked to order by.
//
// The split between this file and its Linux twin is the field's spelling and
// nothing else, exactly as interp/accesstime_darwin.go's is.
func fileChangeTime(info fs.FileInfo) (time.Time, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(st.Ctimespec.Sec, st.Ctimespec.Nsec), true
}

// fileBlocks is how many blocks the file occupies, which is a different
// question from its size: a sparse file and a small one with a large block
// answer them differently, and the sort parameter has a key for each.
func fileBlocks(info fs.FileInfo) (int64, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int64(st.Blocks), true
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"io/fs"
	"syscall"
	"time"
)

// fileAccessTime is when the file was last read. See the darwin file for what
// this answers and why it is read to the nanosecond; the spelling of the
// field is the whole of what differs.
func fileAccessTime(info fs.FileInfo) (time.Time, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(st.Atim.Sec, st.Atim.Nsec), true
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"io/fs"
	"syscall"
	"time"
)

// fileAccessTime is when the file was last read, which `test -N` compares the
// modification time against.
//
// The one thing that differs between the operating systems here is the field's
// spelling — this family calls it `Atimespec` and Linux calls it `Atim` — so
// the split is this line and nothing else. Nanoseconds rather than whole
// seconds: a file written and read inside one second is the ordinary case,
// and a second-resolution comparison answers it wrongly, which is what bash
// 3.2 does and what every other column that has the operator does not.
func fileAccessTime(info fs.FileInfo) (time.Time, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(st.Atimespec.Sec, st.Atimespec.Nsec), true
}

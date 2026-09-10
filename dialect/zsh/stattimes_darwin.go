// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "syscall"

// statTimes is the three timestamps, which this family of Unix spells
// `Atimespec`, `Mtimespec` and `Ctimespec`. It is the whole of what differs
// between the operating systems here; see statsys_unix.go.
func statTimes(st *syscall.Stat_t) (atime, mtime, ctime int64) {
	return st.Atimespec.Sec, st.Mtimespec.Sec, st.Ctimespec.Sec
}

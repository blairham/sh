// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "syscall"

// statTimes is the three timestamps, which this family of Unix spells `Atim`,
// `Mtim` and `Ctim`. It is the whole of what differs between the operating
// systems here; see statsys_unix.go.
func statTimes(st *syscall.Stat_t) (atime, mtime, ctime int64) {
	return st.Atim.Sec, st.Mtim.Sec, st.Ctim.Sec
}

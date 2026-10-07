// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"io/fs"
	"syscall"
)

// compdumpIdentity is the change time in nanoseconds, the inode and the
// device. This kernel spells the first `Ctim`; see compdumpstat_darwin.go.
func compdumpIdentity(fi fs.FileInfo) (ctime int64, ino, dev uint64) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, 0
	}
	return st.Ctim.Nano(), st.Ino, uint64(st.Dev)
}

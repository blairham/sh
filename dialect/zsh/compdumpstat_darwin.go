// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"io/fs"
	"syscall"
)

// compdumpIdentity is the change time in nanoseconds, the inode and the
// device, which the completion dump's key holds beside the size and the
// modification time. This kernel spells the first `Ctimespec`; see
// compdumpstat_linux.go for the other spelling.
func compdumpIdentity(fi fs.FileInfo) (ctime int64, ino, dev uint64) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, 0
	}
	return st.Ctimespec.Nano(), st.Ino, uint64(st.Dev)
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"io/fs"
	"syscall"
)

// hasTwoLinks reports whether a file's hard link count is exactly 2. A
// platform that gives no count answers false, which is the safe answer for
// hasNoSubdirs: the directory is listed.
func hasTwoLinks(info fs.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Nlink == 2
}

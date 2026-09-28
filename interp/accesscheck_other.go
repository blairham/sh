// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package interp

import "os"

// pathAccessible has no access(2) to ask, so it answers from the mode bits —
// which is what every column did before #5049 and is wrong in both directions
// on a system that has a kernel to ask. It is kept as the reading of last
// resort rather than refused, because `-r`, `-w` and `-x` have to answer
// something and a permission model this platform does not have is not a
// reason to call every path unreadable.
//
// times_other and procgroup_other refuse instead, and the difference is worth
// naming: those report a *number* nobody can supply, where this reports a
// yes-or-no that has an approximate answer and a caller with nothing to do
// about it.
func pathAccessible(path string, mode uint32) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return uint32(info.Mode().Perm())&(mode<<6) != 0
}

const (
	accessRead    = 0x4
	accessWrite   = 0x2
	accessExecute = 0x1
)

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package opened

import (
	"io/fs"
	"syscall"
	"time"
)

// atNoFollowSym is AT_SYMLINK_NOFOLLOW, which the standard library does not
// name on Linux. The same value on every Linux architecture.
const atNoFollowSym = 0x100

// traverseChecksEveryLookup: O_PATH is checked at every lookup. See HoldChecked.
const traverseChecksEveryLookup = true

func statAtOnce(dirfd int, name string, follow bool) (fs.FileInfo, error) {
	flags := 0
	if !follow {
		flags = atNoFollowSym
	}
	var st syscall.Stat_t
	if err := fstatat(dirfd, name, &st, flags); err != nil {
		return nil, err
	}
	return newStatInfo(name, &st), nil
}

// accessAtOnce is faccessat with no flags, which every Linux the standard
// library supports passes straight to the kernel: the real identity, as
// access(2) asks.
func accessAtOnce(dirfd int, name string, mode uint32) error {
	return syscall.Faccessat(dirfd, name, mode, 0)
}

func statModTime(st *syscall.Stat_t) time.Time {
	return time.Unix(st.Mtim.Unix())
}

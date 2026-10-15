// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package opened

import (
	"io/fs"
	"syscall"
	"time"
	"unsafe"
)

// fstatat64 and faccessat, by number, for walkat_darwin.go's reason: the
// standard library exposes neither on Darwin. 469 is the 32-bit-inode
// fstatat; 470 is the one whose struct is syscall.Stat_t, which is the one
// os.Stat itself fills. Both are BSD calls, stable since 10.10 and the same on
// every architecture Darwin runs on.
const (
	sysFaccessat  = 466
	sysFstatat64  = 470
	atNoFollowSym = 0x20 // AT_SYMLINK_NOFOLLOW
)

const statAtSupported = true

// traverseChecksEveryLookup is false here: see HoldChecked.
const traverseChecksEveryLookup = false

func statAtOnce(dirfd int, name string, follow bool) (fs.FileInfo, error) {
	p, err := syscall.BytePtrFromString(name)
	if err != nil {
		return nil, err
	}
	flags := 0
	if !follow {
		flags = atNoFollowSym
	}
	var st syscall.Stat_t
	_, _, errno := syscall.Syscall6(sysFstatat64, uintptr(dirfd), uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&st)), uintptr(flags), 0, 0)
	if errno != 0 {
		return nil, errno
	}
	return newStatInfo(name, &st), nil
}

func accessAtOnce(dirfd int, name string, mode uint32) error {
	p, err := syscall.BytePtrFromString(name)
	if err != nil {
		return err
	}
	_, _, errno := syscall.Syscall6(sysFaccessat, uintptr(dirfd), uintptr(unsafe.Pointer(p)),
		uintptr(mode), 0, 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}

func statModTime(st *syscall.Stat_t) time.Time {
	return time.Unix(st.Mtimespec.Unix())
}

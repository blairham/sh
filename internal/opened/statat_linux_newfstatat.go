// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build linux && (amd64 || ppc64 || ppc64le || s390x)

package opened

import (
	"syscall"
	"unsafe"
)

// The architectures whose standard library reaches fstatat as newfstatat and
// does not export it: the call os.Stat itself makes there, into the same
// struct.
const statAtSupported = true

func fstatat(dirfd int, name string, st *syscall.Stat_t, flags int) error {
	p, err := syscall.BytePtrFromString(name)
	if err != nil {
		return err
	}
	_, _, errno := syscall.Syscall6(syscall.SYS_NEWFSTATAT, uintptr(dirfd), uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(st)), uintptr(flags), 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}

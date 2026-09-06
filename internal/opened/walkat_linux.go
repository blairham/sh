// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package opened

import (
	"syscall"
	"unsafe"
)

// oPath is Linux's O_PATH. With O_DIRECTORY it is what O_SEARCH is on Darwin:
// a descriptor that names a place without opening its contents.
const oPath = 0x200000

// traverseFlags opens a directory for *walking through* and nothing else. See
// the Darwin file beside this one for why that distinction is a correctness
// requirement rather than an optimization; the measurement is the same here —
// a 0111 directory answers EACCES to O_RDONLY|O_DIRECTORY, and O_PATH answers
// ok and serves as the dirfd for openat and readlinkat alike.
func traverseFlags() int { return oPath | syscall.O_DIRECTORY }

// pathMax is PATH_MAX, measured the same way as the Darwin file beside this
// one: 4095 bytes opens and 4097 answers ENAMETOOLONG on Linux 6.8. See there
// for why the walk has to impose it itself.
const pathMax = 4096

// maxSymlinks is how many symbolic links this kernel will follow before it
// gives up, measured rather than taken from a header: a chain of 40 opens and
// 41 answers ELOOP on Linux 6.8. Eight more than Darwin, which is why it is per
// platform and not one shared number — a walk that imposed the smaller budget
// everywhere would refuse paths this kernel resolves.
const maxSymlinks = 40

func openat(dirfd int, name string, flags int, perm uint32) (int, error) {
	return syscall.Openat(dirfd, name, flags, perm)
}

// readlinkat is reached by number because the standard library does not export
// it for Linux either, where it exports Openat. The constant is the kernel's
// own, so this is the same arrangement as the Darwin file.
func readlinkat(dirfd int, name string, buf []byte) (int, error) {
	p, err := syscall.BytePtrFromString(name)
	if err != nil {
		return 0, err
	}
	n, _, errno := syscall.Syscall6(syscall.SYS_READLINKAT, uintptr(dirfd), uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0, 0)
	if errno != 0 {
		return 0, errno
	}
	return int(n), nil
}

// walkSupported says whether this platform has a walk at all.
const walkSupported = true

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package opened

import (
	"syscall"
	"unsafe"
)

// The two calls the walk is built on, and the numbers they are reached by.
//
// Named here rather than taken from a header for the reason F_GETPATH is: the
// standard library exposes neither on Darwin — there is no syscall.Openat and
// no syscall.Readlinkat — and both are stable parts of the kernel's interface,
// unchanged since 10.10 and identical on every architecture Darwin runs on
// because they are BSD system calls rather than machine ones. Reached the same
// way the existing fcntl is, which keeps this package's platform surface one
// mechanism rather than two.
const (
	sysOpenat     = 463
	sysReadlinkat = 473
)

// oExec is Darwin's O_EXEC. With O_DIRECTORY it is POSIX's O_SEARCH, which is
// what the walk wants for the directories it only passes through.
const oExec = 0x40000000

// traverseFlags opens a directory for *walking through* and nothing else.
//
// This is not an optimization, it is a correctness requirement, and it was
// measured rather than reasoned about. Passing through a directory needs
// execute permission; opening one for read needs read permission, and the two
// are different bits. A directory that is 0111 — traversable, not readable, and
// an ordinary way to publish one file out of a private tree — is walked
// straight through by the kernel and answers EACCES to O_RDONLY|O_DIRECTORY. A
// resolver built on the readable open would refuse accesses the shell performs
// today, on both platforms, which is a worse failure than the one it is here
// to fix.
//
// O_SEARCH answers ok for exactly that directory, and the descriptor it gives
// serves as the dirfd for openat and readlinkat, which is everything the walk
// asks of it. It cannot be read, and the one thing that needs the readable
// open — a caller whose *target* is the directory — is left to fail with the
// EACCES the kernel would have given anyway.
func traverseFlags() int { return oExec | syscall.O_DIRECTORY }

// pathMax is the longest pathname this kernel will accept, measured rather
// than taken from a header: 1023 bytes opens and 1025 answers ENAMETOOLONG on
// Darwin 25.5.0, so the limit is at 1024 and the test is on the length alone.
//
// The walk needs it because it does not hand the whole path to the kernel any
// more. It hands one component at a time, and a component is never long, so a
// path the kernel would have refused outright would otherwise *open* under a
// policy and fail without one — a gate changing what a shell can do. Checked
// against the caller's own string rather than the absolute form, because that
// is the string the kernel measures: a short relative name under a very deep
// working directory is accepted by both.
const pathMax = 1024

// maxSymlinks is how many symbolic links this platform's own resolver will
// follow before it gives up, measured rather than taken from a header: from a
// root containing none of its own, a chain of 32 opens and 33 answers ELOOP on
// Darwin 25.5.0.
//
// Measured from a symlink-free root on purpose, and the first attempt at it was
// wrong for want of that: a temporary directory on a Mac is under /var, which is
// itself a link, so a chain built there spends one of the budget before it
// starts and reads as 31. The walk keeps the platform's real budget so that a
// path the kernel would refuse is refused here too — a walk with a *larger*
// budget would open chains that fail without a policy, which is a gate changing
// what a shell can do rather than what it may.
//
// TestTheWalkRefusesTheSameChainTheKernelDoes does not trust this constant: it
// finds the kernel's own boundary and requires the walk's to be at the same
// link.
const maxSymlinks = 32

func openat(dirfd int, name string, flags int, perm uint32) (int, error) {
	p, err := syscall.BytePtrFromString(name)
	if err != nil {
		return -1, err
	}
	fd, _, errno := syscall.Syscall6(sysOpenat, uintptr(dirfd), uintptr(unsafe.Pointer(p)),
		uintptr(flags), uintptr(perm), 0, 0)
	if errno != 0 {
		return -1, errno
	}
	return int(fd), nil
}

func readlinkat(dirfd int, name string, buf []byte) (int, error) {
	p, err := syscall.BytePtrFromString(name)
	if err != nil {
		return 0, err
	}
	n, _, errno := syscall.Syscall6(sysReadlinkat, uintptr(dirfd), uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0, 0)
	if errno != 0 {
		return 0, errno
	}
	return int(n), nil
}

// walkSupported says whether this platform has a walk at all.
const walkSupported = true

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package prompttheme

import "syscall"

// wOK is POSIX's W_OK, the write bit of access(2)'s mode argument.
//
// Written down rather than taken from a constant, because the only package
// that carries it as a name is outside this module and this module links
// nothing outside itself — see internal/depsurface. The value is fixed by
// POSIX.1 and is not a platform's to choose.
const wOK = 0x2

// writability asks whether the shell may write into a directory.
//
// access(2) rather than mode bits and a uid comparison, because the mode bits
// are not the answer: a directory can be writable through an access control
// list, through a mount option, or not writable through a read-only
// filesystem, and every one of those is invisible to a stat. The kernel is
// the only thing that knows, and asking it costs one syscall on a path the
// shell is already standing in.
func writability(path string) (writable, known bool) {
	err := syscall.Access(path, wOK)
	if err == nil {
		return true, true
	}
	if err == syscall.EACCES || err == syscall.EROFS || err == syscall.EPERM {
		return false, true
	}
	// Anything else — the directory has gone, a filesystem declined to
	// answer — is not evidence either way, and a prompt that says "not
	// writable" because a lookup failed is the confident wrong answer this
	// repository treats as its worst.
	return false, false
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package gatedfs

import "syscall"

// The two things `zsh/files` needs from the operating system directly, neither
// of which the standard library answers the way this module has to ask. The
// third is the flush itself, which is spelled differently by each family of
// Unix and lives in dialect/zsh/modules/files/sync_darwin.go and dialect/zsh/modules/files/sync_linux.go.

// Writable is the question the default query turns on, and it has to be
// asked of the *process* rather than of the mode bits: whether this user may
// write this file depends on which of owner, group and other they are, on the
// supplementary groups they are in, and on the root exemption. `access` is the
// one call that answers all of that at once.
func Writable(path string) bool {
	return syscall.Access(path, 0o2) == nil
}

// Unlink removes one name and never a directory, which is what makes `rm
// -d` different from `rm`. The standard library's Remove falls back to rmdir
// for a directory, so an empty one would quietly disappear where zsh reports
// what the kernel said — measured, `zf_rm -d emptydir` is `operation not
// permitted`.
func Unlink(path string) error {
	return syscall.Unlink(path)
}

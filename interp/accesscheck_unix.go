// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp

import "syscall"

// pathAccessible asks the kernel whether this process may read, write or
// execute a path, rather than working it out from the mode bits.
//
// The two are not the same question and the mode bits cannot answer it. A
// directory owned by root with mode 755 has the owner's `w` set and an
// ordinary user still cannot write it; a file with mode 400 has no `w` at all
// and an ACL can still grant one. Both directions were measured — see
// [Runner.accessible], where the rows are — and neither is reachable by
// looking at `info.Mode()`.
//
// access(2) rather than a computation over uid, gid and the supplementary
// groups, because the kernel's answer is the one the later `open` will
// honor: it is what carries ACLs, a read-only mount, an immutable flag and
// whatever else the filesystem has that the mode bits do not describe.
//
// **It asks about the *real* uid, and that is stated rather than measured.**
// The distinction only exists for a shell whose real and effective ids differ
// — a setuid binary — which this one is not and which zsh declines to be. A
// probe that would separate the two readings needs a setuid shell to run
// under, so what is written here is the call that was chosen and not a claim
// about what the reference does in a state neither shell can be put into.
func pathAccessible(path string, mode uint32) bool {
	return syscall.Access(path, mode) == nil
}

// The three modes access(2) takes, named here so the caller reads as the test
// it implements. R_OK, W_OK and X_OK are 4, 2 and 1 in POSIX.
const (
	accessRead    = 0x4
	accessWrite   = 0x2
	accessExecute = 0x1
)

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package interp_test

import "syscall"

// testDup2 is the split driver/dup2_linux.go already carries, for the one
// test in this package that needs the call.
//
// Linux/arm64 has no dup2 system call at all — dup3 is the one every Linux
// architecture has — so `syscall.Dup2` does not exist there and a package
// that names it does not compile. See driver/dup2_bsd.go, whose comment says
// exactly this; the reasoning was already written down and this call site
// simply did not use it.
func testDup2(oldfd, newfd int) error { return syscall.Dup3(oldfd, newfd, 0) }

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix && !linux

package interp_test

import "syscall"

// testDup2 is the same call as its Linux spelling, which carries the
// reasoning: there dup3 is the one every architecture has, and here dup2 is.
func testDup2(oldfd, newfd int) error { return syscall.Dup2(oldfd, newfd) }

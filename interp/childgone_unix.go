// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp

import "syscall"

// childIsGone reports whether the kernel no longer knows pid, which for a
// child of this process means it has been reaped: an unreaped child is a
// zombie and still answers. See Runner.awaitAReapedProgramsJob.
func childIsGone(pid int) bool {
	return syscall.Kill(pid, 0) == syscall.ESRCH
}

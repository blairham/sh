// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"syscall"
	"unsafe"
)

// childHasExited reports whether pid, a child of this process, has exited and
// is waiting to be reaped — a zombie — without reaping it. WNOWAIT leaves the
// child for the goroutine whose wait it is, so asking cannot race that wait
// over who collects the status. See Runner.awaitAReapedProgramsJob.
//
// siginfo_t is 128 bytes on every Linux, and si_signo is its first word
// whatever the word size: the kernel sets it to SIGCHLD when it found a child
// to report and leaves it zero when WNOHANG found none.
func childHasExited(pid int) bool {
	const pPID = 1 // P_PID
	var info struct {
		signo int32
		_     [31]int32
	}
	_, _, errno := syscall.Syscall6(syscall.SYS_WAITID, pPID, uintptr(pid),
		uintptr(unsafe.Pointer(&info)), syscall.WEXITED|syscall.WNOHANG|syscall.WNOWAIT, 0, 0)
	return errno == 0 && info.signo == int32(syscall.SIGCHLD)
}

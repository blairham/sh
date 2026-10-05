// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build linux || darwin

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
// Linux and macOS both have waitid with WNOWAIT, and both put si_signo first
// in siginfo_t: the kernel sets it to SIGCHLD when it found a child to report
// and leaves it zero when WNOHANG found none. The structure is 128 bytes on
// Linux and 104 on macOS, so the buffer is the larger, and P_PID is 1 on both.
//
// macOS has the window as well, measured rather than assumed (#5861): with the
// job's goroutine held off its wait, `ps` reports the program in state Z while
// `jobs` lists it running, every time, until this answered there too. And the
// peek leaves the status where it was: the goroutine's own wait still collects
// the 3 that `exit 3` left, which TestAProgramThatExitedUnreapedIsNoticedBefore
// TheTableIsRead reads back on both systems.
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

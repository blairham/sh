// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build darwin || linux

package driver

import (
	"syscall"
	"unsafe"
)

// itimerProf is ITIMER_PROF, the timer the Go runtime's CPU profiler arms. It
// is 2 on both kernels this builds for.
const itimerProf = 2

// profilerRunning reports whether this process's CPU profiler is running, so
// that a PROF is the profiler's own tick rather than one sent from outside.
//
// Asked of the kernel's timer rather than of runtime/pprof, which cannot be
// asked: the runtime arms ITIMER_PROF when a profile starts and disarms it
// when the profile stops, on darwin and on Linux alike, and nothing else in
// this program arms it. Measured 2026-11-04 on macOS arm64: zero before
// pprof.StartCPUProfile, an interval of 10 ms during it, zero after it.
//
// The structure is two timevals, 32 bytes on both 64-bit kernels, and every
// byte of it is zero while the timer is not armed, so no field has to be
// found by its offset.
func profilerRunning() bool {
	var it [4]int64
	if _, _, errno := syscall.Syscall(syscall.SYS_GETITIMER, itimerProf,
		uintptr(unsafe.Pointer(&it)), 0); errno != 0 {
		return false
	}
	return it != [4]int64{}
}

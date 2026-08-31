// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp

import (
	"syscall"
	"time"
)

// processTimes reads the CPU this process and its reaped children have used.
//
// **This is the one number a Runner cannot scope to itself, and saying so is
// the point.** PATH and the working directory are things a Runner *has*, which
// is why lookpath.go refuses to ask the process for them. CPU time is not:
// the kernel accounts it per process, a Runner is not a process, and there is
// no per-Runner figure to report. So `times` in a program that embeds a Runner
// includes whatever else that program has been doing — which is exactly right
// when the program is a shell, and is a limitation worth stating rather than
// papering over when it is not.
//
// Children are the ones this process has *waited for*, which is what
// RUSAGE_CHILDREN means and what a shell reports: a background job still
// running has contributed nothing yet.
func processTimes() (self, children cpuTime, ok bool) {
	var s, c syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &s); err != nil {
		return cpuTime{}, cpuTime{}, false
	}
	if err := syscall.Getrusage(syscall.RUSAGE_CHILDREN, &c); err != nil {
		return cpuTime{}, cpuTime{}, false
	}
	return rusageTime(s), rusageTime(c), true
}

func rusageTime(r syscall.Rusage) cpuTime {
	return cpuTime{
		user:   time.Duration(r.Utime.Nano()),
		system: time.Duration(r.Stime.Nano()),
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package cross_test

import "syscall"

// prSetDumpable is prctl's, fixed by the kernel's ABI.
const prSetDumpable = 4

// refuseACoreDump keeps a shell half that dies of a core-dumping signal from
// being dumped: the resource limit for a dump the kernel writes itself, and
// the flag for one a piped core_pattern hands to a crash reporter, which
// ignores the limit and costs a race-instrumented binary seconds a death on a
// busy runner. See driver's writeNoCore for the measurement.
func refuseACoreDump() {
	_ = syscall.Setrlimit(syscall.RLIMIT_CORE, &syscall.Rlimit{})
	_, _, _ = syscall.Syscall(syscall.SYS_PRCTL, prSetDumpable, 0, 0)
}

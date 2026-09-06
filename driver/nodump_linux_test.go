// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package driver_test

import "syscall"

// prSetDumpable is prctl's PR_SET_DUMPABLE. Written out rather than taken from
// a dependency: it is one number, fixed by the kernel's ABI, and these tests
// have no other reason to reach outside the standard library.
const prSetDumpable = 4

// refuseToBeDumped tells the kernel this process is not to be dumped, which is
// the half of writeNoCore that a piped core_pattern does not ignore.
//
// The resource limit is consulted only for a dump the kernel writes itself, so
// a machine whose core_pattern names a crash reporter dumps anyway — measured
// at 156ms per death against 1ms, on a `--cpus=1` container with a
// race-instrumented binary and nothing else running. The flag has no such
// exception: there is no image to write and no helper to start.
//
// Ignoring the error is the right answer and not laziness. This is a guard
// against a slow death, not a requirement for a correct one: a kernel that
// refuses it leaves the tests measuring what they measured before, which is
// the state this is an improvement on rather than a regression from.
func refuseToBeDumped() {
	_, _, _ = syscall.Syscall(syscall.SYS_PRCTL, prSetDumpable, 0, 0)
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"syscall"
)

// The region every placement rule in this package allocates from: the floor
// below which a script's own `exec 3>out` numbers live, up to the top of the
// table.
//
// Both numbers are interp's, and unexported there — firstProcSubFd and
// topOfTheDescriptorTable. Written again here because the cases in
// procsubfdplacement_test.go already name the second one in so many words,
// and because what this file needs them for is not the rule itself but the
// question "was this process handed a table the rule can still be seen in".
const (
	placementFloor = 10
	placementTop   = 63
)

// highestInheritedFd is how far up the table the sweep below looks.
//
// A number the host handed us above this one is above everything any rule
// here allocates from, so leaving it open changes no measurement in this
// package. A bound rather than the open-file limit because that limit is
// 1048576 on a stock Linux and the sweep is one fcntl per number.
const highestInheritedFd = 1024

// quietFdTableEnv marks the run that has already been given a quiet table, so
// that the restart below happens at most once.
const quietFdTableEnv = "SH_TEST_QUIET_FD_TABLE"

// restartWithAQuietDescriptorTable gives this test binary the descriptor
// table its measurements assume, by starting itself again without the numbers
// it was handed.
//
// # Why a test binary has to care
//
// The placement cases in this package ask where a process substitution's far
// end lands, and the answer comes from the kernel's table for the **whole
// binary**. That table is not ours alone: whatever started us is free to hand
// down open descriptors, and `go test ./interp/` imports no driver, so nothing
// has moved the Go runtime's own off the low numbers either.
//
// On a developer laptop the table is nearly empty and every rule in the file
// is visible in it. On the macOS CI runner it is not: the binary was handed
// some seventy open descriptors, the whole of [10,63] was taken, and nine
// cases failed at once reporting numbers in the seventies (#4459). Nothing in
// the shell had moved — a required check was failing on the table the runner
// happened to have open, which trains everyone to rerun it, and a rerun is
// indistinguishable from one that hid something real.
//
// # Why a restart and not a looser assertion
//
// Because the cases are not all rescuable by relative bounds. Three of them
// compare a run against what the kernel says is free and would only need the
// bound expressed against that — but a crowded table also puts this shell's
// *own* plumbing in the region it publishes from, and it takes the nesting
// release out of reach entirely: `substEndCandidates` offers sixty-four
// numbers from the dialect's base, and a number above all of them can neither
// be wished for nor released, so `cat <(echo <(true))` answers three apart
// rather than on the nose. A bound loose enough to pass there is loose enough
// to pass for the fault those cases exist to catch.
//
// So the table is made quiet instead, which is the other half of the same
// discipline: the environment is an input, and this one is pinned rather than
// measured around.
//
// # How
//
// Close-on-exec does the one thing that cannot be done in place. At this point
// in the run a descriptor above 2 is either one the host handed down or one
// the Go runtime opened for itself, and nothing here can tell those apart —
// closing the runtime's poller would take the process with it. After an exec
// they *are* distinguishable, because the runtime marks everything it opens
// close-on-exec: the ones that survive are exactly the inherited ones, so
// marking every number in sight and starting again leaves stdio, and the new
// runtime's own descriptors on top of an otherwise empty table.
//
// This replaces the process rather than starting a child beside it, so there
// is one pid, one set of streams, one exit status, and nothing to orphan if
// the run is killed. The rule that sends syscall.Exec to `driver` is about a
// Runner embedded in somebody else's program; this is the test binary
// restarting itself before any test has run, which is the one place in the
// tree where replacing the process is the whole point.
//
// A failure is not one. If the exec is refused the run goes ahead with the
// table it was given and the cases that care report the numbers they got,
// which is exactly what #4459 looked like — visible, and not a silent pass.
func restartWithAQuietDescriptorTable() {
	if os.Getenv(quietFdTableEnv) != "" {
		return
	}
	if !descriptorTableIsCrowded() {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	for fd := 3; fd < highestInheritedFd; fd++ {
		_, _, _ = syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd),
			uintptr(syscall.F_SETFD), uintptr(syscall.FD_CLOEXEC))
	}
	_ = syscall.Exec(exe, os.Args, append(os.Environ(), quietFdTableEnv+"=1"))
}

// descriptorTableIsCrowded says whether anything is sitting in the region the
// placement rules allocate from.
//
// Anything at all, rather than a count with room to spare: a run that starts
// with that region to itself is the one the cases were written against, and on
// a quiet machine this is false and nothing restarts.
func descriptorTableIsCrowded() bool {
	for fd := placementFloor; fd <= placementTop; fd++ {
		if fdIsOpen(fd) {
			return true
		}
	}
	return false
}

// fdIsOpen says whether this process holds the number fd.
//
// F_GETFD is the cheapest question that distinguishes a live entry from an
// empty one, and it takes nothing: a probe that duplicated the number to find
// out would be holding it while the next one is asked about.
func fdIsOpen(fd int) bool {
	_, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd),
		uintptr(syscall.F_GETFD), 0)
	return errno == 0
}

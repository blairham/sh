// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"fmt"
	"syscall"

	"github.com/blairham/sh/interp"
)

// rlimitOf maps a resource this shell names to the number the kernel does.
//
// The translation lives here rather than in interp for the reason setUmask
// does: a limit is process state, and the package that interprets a script has
// no business reaching for syscall to change it. It also keeps interp building
// where these constants do not exist.
//
// Like the mask, a limit is outside the gate and the event stream on purpose;
// umask.go carries the reasoning at length, and it holds here unchanged. A
// limit names no path and no program, it changes what a later action is
// allowed to do rather than performing one, and the hook itself is where a
// program that wants a say installs it — which can refuse, where an event
// could only report.
var rlimitOf = map[interp.Resource]int{
	interp.ResourceCore:         syscall.RLIMIT_CORE,
	interp.ResourceData:         syscall.RLIMIT_DATA,
	interp.ResourceFileSize:     syscall.RLIMIT_FSIZE,
	interp.ResourceOpenFiles:    syscall.RLIMIT_NOFILE,
	interp.ResourceStack:        syscall.RLIMIT_STACK,
	interp.ResourceCPUTime:      syscall.RLIMIT_CPU,
	interp.ResourceAddressSpace: syscall.RLIMIT_AS,
}

// The three the portable syscall package does not name — locked memory, the
// resident set, the process count — join the table where the platform file
// looked their numbers up, and stay honestly absent where none did.
func init() {
	for res, id := range map[interp.Resource]int{
		interp.ResourceLockedMemory: rlimitLockedMemory,
		interp.ResourceResidentSet:  rlimitResidentSet,
		interp.ResourceProcesses:    rlimitProcesses,
	} {
		if id >= 0 {
			rlimitOf[res] = id
		}
	}
}

// getRlimit reads one limit, in the kernel's own units.
//
// A resource this build has no number for is reported as such rather than
// answered with a wrong one: the set above is what every platform this builds
// for agrees on, and the ones missing from it — locked memory, resident set,
// process count — are spelled differently or not at all from one Unix to the
// next.
//
// # Open files is read faithfully and is still not what a child gets
//
// The Go runtime raises this process's RLIMIT_NOFILE soft limit before any
// code here runs, and os/exec hands children the value the process had
// *before* it did. So `ulimit -n` reports what this process runs under, which
// is not the limit the commands it runs will run under — backwards for a
// shell, whose whole reason for having the builtin is to say what its
// commands get.
//
// Measured on both platforms, and it is worse on Linux. In a container
// started with soft 1024 and hard 1048576: the Go process reads 1048575, a
// `/bin/sh` child sees 1024, and a non-Go `/bin/sh` in the same container
// sees 1024. With soft and hard already equal there is nothing to raise and
// nothing goes wrong, which is how a casual check misses it.
//
// Four ways out were measured and three do not exist:
//
//   - Recording it at startup is impossible. The raise happens in a syscall
//     package init, before anything here runs.
//   - Recovering it afterwards has no portable form. The runtime's copy is
//     unexported, /proc/self/limits reports the raised value, and a Go child
//     re-raises its own — only a *non-Go* child sees the original, which is
//     why this went unnoticed and which costs a process per call to ask.
//   - Calling Setrlimit at startup so the two agree does work, and is the
//     wrong thing: on Linux it would hand every child 1048575 in place of
//     1024, which is exactly what a low soft limit exists to prevent. Go's
//     own syscall/rlimit.go says so — some systems set an artificially low
//     soft limit for code that uses select and its hard-coded maximum
//     descriptor. Silently raising a child's limit a thousandfold breaks
//     programs that run fine under bash.
//
// So it is reported as read and written down here. The runtime leaves a tell
// for anyone who needs to detect the situation: it sets Cur to Max-1, so
// `Cur == Max-1` means the value is an artifact rather than anybody's real
// limit. That is enough to notice and not enough to recover, which is the
// whole of why this stands.
//
// The *set* path is unaffected and correct: Setrlimit stores nothing in the
// runtime's copy, so `ulimit -n 256` gives children 256. Only reading is
// wrong, and only for this one resource — every other matched the panel
// exactly, and so did this one's hard limit.
func getRlimit(res interp.Resource) (int64, int64, error) {
	id, ok := rlimitOf[res]
	if !ok {
		return 0, 0, fmt.Errorf("this build has no such limit")
	}
	var l syscall.Rlimit
	if err := syscall.Getrlimit(id, &l); err != nil {
		return 0, 0, err
	}
	return fromRlim(l.Cur), fromRlim(l.Max), nil
}

func setRlimit(res interp.Resource, soft, hard int64) error {
	id, ok := rlimitOf[res]
	if !ok {
		return fmt.Errorf("this build has no such limit")
	}
	return syscall.Setrlimit(id, &syscall.Rlimit{Cur: toRlim(soft), Max: toRlim(hard)})
}

// rlimInfinity is this platform's "no limit", which is not the same number
// everywhere: Linux spells it -1 and Darwin the largest int64.
//
// Reached through an int64 variable rather than converted from the constant
// directly. `uint64(syscall.RLIM_INFINITY)` is a compile error on Linux, where
// the constant is negative and untyped — which is what the Linux cross-build
// caught, on a machine where the constant is positive and it built cleanly.
var (
	rlimInfinityInt64 int64 = syscall.RLIM_INFINITY
	rlimInfinity            = uint64(rlimInfinityInt64)
)

// fromRlim and toRlim convert between the kernel's "no limit" and this
// shell's. interp names its own so that a caller with a different one — a test
// double, a sandbox — maps it here rather than being told what the number is.
func fromRlim(v uint64) int64 {
	if v == rlimInfinity {
		return interp.RlimitInfinity
	}
	return int64(v)
}

func toRlim(v int64) uint64 {
	if v == interp.RlimitInfinity {
		return rlimInfinity
	}
	return uint64(v)
}

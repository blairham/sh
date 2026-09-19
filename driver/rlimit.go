// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"fmt"
	"sort"
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

// The eight the portable syscall package does not name join the table where
// the platform file looked their numbers up, and stay honestly absent where
// none did.
//
// The first three — locked memory, the resident set, the process count —
// exist on every platform this builds for and are only spelled differently.
// The five after them are Linux's alone, and their absence is load-bearing
// rather than a gap: `ulimit -a` asks hasRlimit and leaves the row out.
func init() {
	for res, id := range map[interp.Resource]int{
		interp.ResourceLockedMemory:       rlimitLockedMemory,
		interp.ResourceResidentSet:        rlimitResidentSet,
		interp.ResourceProcesses:          rlimitProcesses,
		interp.ResourceFileLocks:          rlimitFileLocks,
		interp.ResourcePendingSignals:     rlimitPendingSignals,
		interp.ResourceMessageQueues:      rlimitMessageQueues,
		interp.ResourceSchedulingPriority: rlimitSchedulingPriority,
		interp.ResourceRealtimePriority:   rlimitRealtimePriority,
		interp.ResourceRealtimeTime:       rlimitRealtimeTime,
	} {
		if id >= 0 {
			rlimitOf[res] = id
		}
	}
}

// hasRlimit reports whether this kernel has a limit at all, which is what
// tells `ulimit -a` a row belongs in the table. See Runner.HasRlimit.
func hasRlimit(res interp.Resource) bool {
	if res == interp.ResourcePipeBuffer {
		return pipeBufferBytes > 0
	}
	_, ok := rlimitOf[res]
	return ok
}

// rlimitOrder is the limits this kernel has in the order it numbers them, one
// entry per number. See Runner.RlimitOrder, which is what reads it and why the
// sequence rather than the set is the answer.
//
// Where one number carries two names — this platform's headers number the
// resident set and the address space alike, and the map above holds both — the
// entry is the address space. Measured rather than chosen: the shell that
// lists in this order prints `-v: address space` for that number on such a
// kernel and has no `-m` row at all, while on a kernel that numbers the two
// apart it prints both.
func rlimitOrder() []interp.Resource {
	byNumber := make(map[int]interp.Resource, len(rlimitOf))
	for res, id := range rlimitOf {
		if prev, taken := byNumber[id]; taken && prev == interp.ResourceAddressSpace {
			continue
		}
		byNumber[id] = res
	}
	numbers := make([]int, 0, len(byNumber))
	for id := range byNumber {
		numbers = append(numbers, id)
	}
	sort.Ints(numbers)
	order := make([]interp.Resource, 0, len(numbers))
	for _, id := range numbers {
		order = append(order, byNumber[id])
	}
	return order
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
	// The pipe buffer is the platform's own number rather than a limit, and
	// it is read through this hook so that the package interpreting a script
	// never looks a platform constant up for itself. It has no soft and hard
	// halves, so the one number answers both.
	if res == interp.ResourcePipeBuffer {
		if pipeBufferBytes == 0 {
			return 0, 0, fmt.Errorf("this build has no such limit")
		}
		return pipeBufferBytes, pipeBufferBytes, nil
	}
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
	// Nothing can change the pipe buffer, and the kernel's own word for the
	// attempt is what the shell that lists it prints: `ulimit: pipe size:
	// cannot modify limit: Invalid argument`, measured 2026-09-18 on bash
	// 5.3.20 for every operand including the value the row already holds.
	if res == interp.ResourcePipeBuffer {
		return syscall.EINVAL
	}
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

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
var rlimitOf = map[interp.Resource]int{
	interp.ResourceCore:         syscall.RLIMIT_CORE,
	interp.ResourceData:         syscall.RLIMIT_DATA,
	interp.ResourceFileSize:     syscall.RLIMIT_FSIZE,
	interp.ResourceOpenFiles:    syscall.RLIMIT_NOFILE,
	interp.ResourceStack:        syscall.RLIMIT_STACK,
	interp.ResourceCPUTime:      syscall.RLIMIT_CPU,
	interp.ResourceAddressSpace: syscall.RLIMIT_AS,
}

// getRlimit reads one limit, in the kernel's own units.
//
// A resource this build has no number for is reported as such rather than
// answered with a wrong one: the set above is what every platform this builds
// for agrees on, and the ones missing from it — locked memory, resident set,
// process count — are spelled differently or not at all from one Unix to the
// next.
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

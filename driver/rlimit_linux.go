// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

// The limits the portable syscall package leaves unnamed, by the numbers
// this kernel's own headers give them. Facts about the platform, not
// expression: linux/resource.h numbers RLIMIT_RSS 5, RLIMIT_NPROC 6 and
// RLIMIT_MEMLOCK 8.
const (
	rlimitResidentSet  = 5
	rlimitProcesses    = 6
	rlimitLockedMemory = 8
)

// The pipe buffer, which is not a limit and is listed beside them by two
// shells all the same. Measured 2026-09-18 in the panel's Alpine image: bash
// writes it in 512-byte blocks and prints 8 there.
const pipeBufferBytes = 4096

// And six this kernel has that the BSDs do not, by the numbers
// asm-generic/resource.h gives them. Go's syscall package names none of the
// five on any platform, so they are written out here for the same reason the
// three above are.
const (
	rlimitFileLocks          = 10
	rlimitPendingSignals     = 11
	rlimitMessageQueues      = 12
	rlimitSchedulingPriority = 13
	rlimitRealtimePriority   = 14
	rlimitRealtimeTime       = 15
)

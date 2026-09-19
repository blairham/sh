// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

// The limits the portable syscall package leaves unnamed, by the numbers
// this kernel's own headers give them. Facts about the platform, not
// expression: sys/resource.h numbers RLIMIT_RSS 5, RLIMIT_MEMLOCK 6 and
// RLIMIT_NPROC 7 here.
const (
	rlimitResidentSet  = 5
	rlimitLockedMemory = 6
	rlimitProcesses    = 7
)

// The pipe buffer, which is not a limit and is listed beside them by two
// shells all the same. Measured 2026-09-18: bash writes it in 512-byte blocks
// and prints 1 here, and ksh93 writes it in bytes and prints 512.
const pipeBufferBytes = 512

// The six Linux limits this kernel does not have. Not "unimplemented" and
// not zero: there is no such limit here, so `ulimit -a` leaves the row out
// entirely — which is what bash 5.3, zsh and dash were each measured doing on
// this platform, each printing its Linux table without them.
const (
	rlimitFileLocks          = -1
	rlimitPendingSignals     = -1
	rlimitMessageQueues      = -1
	rlimitSchedulingPriority = -1
	rlimitRealtimePriority   = -1
	rlimitRealtimeTime       = -1
)

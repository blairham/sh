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

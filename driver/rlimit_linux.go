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

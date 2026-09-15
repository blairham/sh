// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin && !linux

package driver

// A platform whose numbers were not looked up keeps the honest refusal: a
// negative id is left out of the table entirely.
const (
	rlimitResidentSet  = -1
	rlimitProcesses    = -1
	rlimitLockedMemory = -1
)

// And the five Linux-only ones, absent here for the same reason.
const (
	rlimitFileLocks          = -1
	rlimitPendingSignals     = -1
	rlimitMessageQueues      = -1
	rlimitSchedulingPriority = -1
	rlimitRealtimePriority   = -1
)

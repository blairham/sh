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

// The pipe buffer is the same kind of unlooked-up number, and zero leaves the
// row out rather than printing a size nobody measured.
const pipeBufferBytes = 0

// And the six Linux-only limits, absent here for the same reason.
const (
	rlimitFileLocks          = -1
	rlimitPendingSignals     = -1
	rlimitMessageQueues      = -1
	rlimitSchedulingPriority = -1
	rlimitRealtimePriority   = -1
	rlimitRealtimeTime       = -1
)

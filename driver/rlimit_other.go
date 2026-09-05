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

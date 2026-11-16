// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix && !darwin && !linux

package driver_test

// platformEndsTheShellFromOutside adds nothing on a kernel whose table was
// not measured.
var platformEndsTheShellFromOutside []namedSignal

// trappedProfFromOutside and standardStreamCopyArrivesApart are measured on
// darwin and Linux only; see those files.
const (
	trappedProfFromOutside         = "after\n"
	standardStreamCopyArrivesApart = false
)

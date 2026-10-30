// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix && !darwin && !linux

package driver_test

// platformEndsTheShellFromOutside adds nothing on a kernel whose table was
// not measured.
var platformEndsTheShellFromOutside []namedSignal

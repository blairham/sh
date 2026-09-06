// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix && !linux

package driver_test

// refuseToBeDumped has nothing to do where the kernel has no equivalent knob.
//
// macOS is the platform this covers, and it does not need one: core dumps are
// off there unless somebody turned them on, there is no piped-core_pattern
// equivalent for the resource limit to be ignored on, and the deaths these
// tests measure are 8ms there against the 4 seconds a Linux runner reached.
func refuseToBeDumped() {}

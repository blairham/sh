// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package repl

import "syscall"

// The ioctl numbers differ by platform, which is the whole of what is
// platform-specific here.
const (
	tcGets = syscall.TCGETS
	tcSets = syscall.TCSETS
)

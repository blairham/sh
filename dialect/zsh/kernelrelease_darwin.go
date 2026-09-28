// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "syscall"

// kernelRelease is what `uname -r` answers, which is the number this
// platform's `$OSTYPE` carries after the name — `darwin25.6.0`.
//
// Empty where the kernel will not say, which leaves `$OSTYPE` as the bare
// platform name: a shorter answer rather than a wrong one, and still the
// prefix every script that branches on the parameter reads.
func kernelRelease() string {
	release, err := syscall.Sysctl("kern.osrelease")
	if err != nil {
		return ""
	}
	return release
}

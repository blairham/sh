// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package tty

// A platform whose terminals this package does not know about has none it can
// measure, and says so rather than guessing — which is the same answer
// isTerminalFd gives there, and for the same reason.
func sizeOfFd(uintptr) (rows, cols int) { return 0, 0 }

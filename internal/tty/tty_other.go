// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package tty

// A platform whose terminals this package does not know about has none it can
// recognize, and says so rather than guessing from a file mode — which is the
// guess this package exists to replace.
func isTerminalFd(uintptr) bool { return false }

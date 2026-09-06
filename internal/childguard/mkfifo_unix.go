// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package childguard

import "syscall"

// mkfifo makes a named pipe, which this package's own test needs so that the
// stray it leaves is the same shape as the ones it was written to find.
func mkfifo(path string) error { return syscall.Mkfifo(path, 0o600) }

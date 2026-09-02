// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp

import "syscall"

// mkfifo makes a named pipe.
//
// 0600: what passes through it is this shell's data, and a pipe anyone on the
// machine can open is a pipe anyone can read.
func mkfifo(path string) error { return syscall.Mkfifo(path, 0o600) }

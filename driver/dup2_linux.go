// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package driver

import "syscall"

// dup2 puts a descriptor on a chosen number, closing whatever was there, and
// leaves the copy open across an exec.
//
// Atomically, which is the reason this is not a close followed by an
// F_DUPFD: a shell with a background job has goroutines that open files of
// their own, and one of them taking the number in the window between the two
// calls would put a job's file where the script's descriptor should be.
//
// Two spellings because the system call was split. dup2 does not exist on
// every Linux architecture — arm64 and riscv64 have only dup3, which is the
// same call with a flag word — while the BSDs and macOS have only dup2. A
// zero flag word makes dup3 dup2 exactly, including clearing close-on-exec on
// the new descriptor.
func dup2(oldfd, newfd int) error { return syscall.Dup3(oldfd, newfd, 0) }

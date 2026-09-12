// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package repl

import "syscall"

const probeTcGets = syscall.TIOCGETA

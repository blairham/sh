// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package repl

import "syscall"

const probeTcGets = syscall.TCGETS

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package tests_test

import "syscall"

const probeTcGets = syscall.TCGETS

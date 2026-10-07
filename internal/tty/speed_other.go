// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package tty

func outputSpeedFd(uintptr) int { return 0 }

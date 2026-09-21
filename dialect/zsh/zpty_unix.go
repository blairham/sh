// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build darwin || linux

package zsh

// zptySupported is whether this build can open a pseudo-terminal pair and
// change its line discipline. It matches the platform set internal/pty opens
// a pair on, because that package is where the pair comes from.
const zptySupported = true

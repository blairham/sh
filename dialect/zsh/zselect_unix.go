// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package zsh

// zselectSupported is whether this build can ask the kernel which descriptors
// are ready. It matches the platform set internal/fdset answers on, because
// that package is where the question is asked.
const zselectSupported = true

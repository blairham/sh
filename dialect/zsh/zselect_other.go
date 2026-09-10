// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd)

package zsh

// A platform where internal/fdset has no descriptor set to wait on, so this
// module is not registered at all. See registerZselectModule.
const zselectSupported = false

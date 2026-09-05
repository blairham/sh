// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd)

package interp

// readableNow cannot be asked on a system without a descriptor set, so every
// stream counts as ready — the same answer inputWaiting gives anything else
// it cannot ask.
func readableNow(int) bool { return true }

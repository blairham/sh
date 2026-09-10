// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package interp

// seekCurrent has no answer on a platform whose descriptors this package does
// not reach through a numbered table, so every descriptor is "not one" — the
// same answer a number nothing is open at gets, and the honest one rather
// than a position invented for it.
func seekCurrent(int) (int64, bool) { return 0, false }

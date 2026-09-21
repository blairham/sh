// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package prompttheme

// writability declines to answer where there is no access(2).
//
// Not "writable": a platform this tree cannot ask must draw the plain state
// rather than claim either way, and a segment that reported every directory
// writable would be reporting a fact it never checked.
func writability(string) (writable, known bool) { return false, false }

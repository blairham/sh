// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin && !linux

package opened

import "errors"

// No walk on these platforms, which is a statement about them rather than a
// gap left for later, and it is the same statement pathOfFd already makes
// beside it: there is no way here to ask the kernel what an open reached, so
// the gate matches the name alone as every platform did before #922. A walk
// needs openat and readlinkat, and the standard library exports neither
// portably; reaching them by number is exactly the kind of guess that must not
// be made on a platform nobody is measuring.
//
// walkUnsupported is returned rather than a wrong answer, and Verified reads it
// as "take the ordinary open" — so a build for one of these platforms behaves
// as it does today rather than failing to open files.
var walkUnsupported = errors.New("opened: no path walk on this platform")

const maxSymlinks = 0

// pathMax is not zero here even though the walk never runs, because a limit
// of zero reads as "every path is too long" and a constant that would be a
// landmine if it were ever reached is worse than one that is merely unused.
const pathMax = 1 << 20

func traverseFlags() int { return 0 }

func openat(int, string, int, uint32) (int, error) { return -1, walkUnsupported }

func readlinkat(int, string, []byte) (int, error) { return 0, walkUnsupported }

// walkSupported says whether this platform has a walk at all.
const walkSupported = false

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package fdset

import (
	"syscall"
	"time"
)

// nfdbits is how many descriptors one word of a descriptor set holds.
const nfdbits = 32

// selectRead is the one call whose shape differs between the systems that
// have it: here it reports only whether it failed.
func selectRead(nfd int, set *syscall.FdSet, tv *syscall.Timeval) error {
	return syscall.Select(nfd, set, nil, nil, tv)
}

// timeval is a duration in the shape this system's select wants it. The two
// fields are not the same width everywhere, which is why this is here and not
// beside its one caller.
func timeval(d time.Duration) syscall.Timeval {
	if d < 0 {
		d = 0
	}
	return syscall.Timeval{
		Sec:  int64(d / time.Second),
		Usec: int32(d % time.Second / time.Microsecond),
	}
}

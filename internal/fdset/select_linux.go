// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package fdset

import (
	"syscall"
	"time"
)

// nfdbits is how many descriptors one word of a descriptor set holds.
const nfdbits = 64

// selectRead is the one call whose shape differs between the systems that
// have it: here it also counts the descriptors that came back ready, which
// the set itself already says.
func selectRead(nfd int, set *syscall.FdSet, tv *syscall.Timeval) error {
	_, err := syscall.Select(nfd, set, nil, nil, tv)
	return err
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
		Usec: int64(d % time.Second / time.Microsecond),
	}
}

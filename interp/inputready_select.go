// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package interp

import (
	"errors"
	"syscall"
)

// readableNow asks the kernel whether a read on fd would return at once,
// waiting no time at all for the answer.
//
// The descriptor is not read from, which is the whole point: the caller wants
// to know whether input is waiting and a read is what would consume it.
func readableNow(fd int) bool {
	var set syscall.FdSet
	if fd/nfdbits >= len(set.Bits) {
		// Past what a descriptor set can name. Nothing here can ask about
		// it, so it is treated as ready like any other stream that cannot
		// be asked.
		return true
	}
	tv := syscall.Timeval{}
	for {
		set = syscall.FdSet{}
		set.Bits[fd/nfdbits] |= 1 << (uint(fd) % nfdbits)
		err := selectRead(fd+1, &set, &tv)
		if errors.Is(err, syscall.EINTR) {
			// A signal arrived while the question was being asked. The
			// question is unchanged, so it is asked again.
			continue
		}
		if err != nil {
			return true
		}
		return set.Bits[fd/nfdbits]&(1<<(uint(fd)%nfdbits)) != 0
	}
}

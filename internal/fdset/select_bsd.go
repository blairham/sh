// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package fdset

import "syscall"

// nfdbits is how many descriptors one word of a descriptor set holds.
const nfdbits = 32

// selectRead is the one call whose shape differs between the systems that
// have it: here it reports only whether it failed.
func selectRead(nfd int, set *syscall.FdSet, tv *syscall.Timeval) error {
	return syscall.Select(nfd, set, nil, nil, tv)
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package interp

import "syscall"

// nfdbits is how many descriptors one word of a descriptor set holds.
const nfdbits = 64

// selectRead is the one call whose shape differs between the systems that
// have it: here it also counts the descriptors that came back ready, which
// the set itself already says.
func selectRead(nfd int, set *syscall.FdSet, tv *syscall.Timeval) error {
	_, err := syscall.Select(nfd, set, nil, nil, tv)
	return err
}

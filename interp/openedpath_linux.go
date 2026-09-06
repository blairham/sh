// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"strconv"
	"strings"
)

// deletedSuffix is what /proc appends to the path of a file whose last link
// has gone. The name it decorates no longer reaches the object, so there is
// nothing for a rule about names to say about it, and it is read here as the
// nameless case rather than stripped — stripping would produce a name that
// reaches somewhere else entirely if something has since taken it.
const deletedSuffix = " (deleted)"

// pathOfFd asks the kernel for the path of an open descriptor, through the
// per-descriptor symbolic links /proc keeps.
//
// The nameless cases arrive as strings that are not paths — `pipe:[12345]`,
// `socket:[…]`, `anon_inode:…` — and the absolute-path test below is what
// separates them, rather than a list of prefixes a later kernel could add to.
func pathOfFd(fd uintptr) (string, bool) {
	target, err := os.Readlink("/proc/self/fd/" + strconv.FormatUint(uint64(fd), 10))
	if err != nil || !strings.HasPrefix(target, "/") || strings.HasSuffix(target, deletedSuffix) {
		return "", false
	}
	return target, true
}

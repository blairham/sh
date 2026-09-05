// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp

import "os"

// firstExtraFd is the descriptor os/exec's ExtraFiles starts at: entry i of
// that slice becomes descriptor 3+i in the child, because 0, 1 and 2 are the
// named streams and are carried separately.
const firstExtraFd = 3

// maxInheritedFd bounds the table that gets rebuilt. It is a guard rather
// than a rule about shells: the slice has one entry per descriptor up to the
// highest one held, so a script that says `exec 1000000>f` would otherwise
// ask for a million entries to describe one open file.
//
// 1024 is well clear of anything the panel agrees on. dash and zsh do not
// read a multi-digit descriptor number at all — `exec 250>f` is a command
// named 250 in both — and bash refuses one near the process limit outright,
// so no shell measured can put a descriptor this high in play. That we
// accept such a number where bash rejects it is a separate divergence; this
// only declines to build a table for it.
const maxInheritedFd = 1024

// childFiles rebuilds, for an external child, the descriptor table this shell
// holds beyond the three named streams.
//
// The boundary has to be rebuilt by hand because Go opens everything
// close-on-exec, so a descriptor the script put on 3 with `exec 3>f` reaches
// no child at all: `exec 3>f; /bin/sh -c 'echo child >&3'` wrote nothing and
// said "Bad file descriptor". Every shell in the panel but ksh93 hands the
// descriptor over, which is what the flock and shared-log idioms rely on.
//
// The layout is the whole of it, and it is measured rather than assumed:
// with 3 and 4 unopened, `exec 5>f; /bin/sh -c 'echo x >&5'` writes in bash,
// dash and zsh, and 3 and 4 are *closed* in that child rather than shifted
// down. So the number is preserved and a gap is a hole: entry fd-3 holds the
// file, and a nil entry is a descriptor closed in the child, which is exactly
// what os/exec does with a nil.
//
// Only a real file can cross. A Runner embedded in another program may have a
// buffer or a pipe of the caller's behind a descriptor, and there is no
// number to hand a child for one of those; those entries stay nil, which
// leaves them closed there as they were before.
func (r *Runner) childFiles() []*os.File {
	highest := 0
	for fd, v := range r.fds {
		if fd < firstExtraFd || fd > maxInheritedFd {
			continue
		}
		if _, ok := v.(*os.File); ok && fd > highest {
			highest = fd
		}
	}
	if highest == 0 {
		return nil
	}
	files := make([]*os.File, highest-firstExtraFd+1)
	for fd, v := range r.fds {
		if fd < firstExtraFd || fd > highest {
			continue
		}
		if f, ok := v.(*os.File); ok {
			files[fd-firstExtraFd] = f
		}
	}
	return files
}

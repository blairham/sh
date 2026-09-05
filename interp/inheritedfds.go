// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "context"

// firstExtraFd is the descriptor the table beyond the three named streams
// starts at, in both directions. Outbound it is os/exec's ExtraFiles layout —
// entry i of that slice becomes descriptor 3+i in the child, because 0, 1 and
// 2 are the named streams and are carried separately — and InheritedFiles is
// read the same way, so one number describes both halves.
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

// publishInheritedFds puts the descriptors this shell was started with into
// the table, once, before the first statement runs.
//
// A shell models the descriptor table, so a descriptor it did not open does
// not exist until something says so: `echo hello | sh 3<&0 script` left
// `exec <&3` saying "3: bad file descriptor" where every shell in the panel
// dups it silently and reads through it. Nothing in the script opened that
// descriptor and nothing in the script can, which is why it has to arrive as
// a fact rather than as an action.
//
// The layout is childFiles' layout read backwards — entry i is descriptor
// 3+i, and a nil entry is a number nothing was inherited on — so the two
// halves of the boundary describe the table the same way, and a descriptor
// that came in on 5 goes out on 5.
//
// Once: the guard is copied by clone along with everything else, so a
// subshell inherits the table it was cloned with rather than publishing over
// it, and a front end feeding the shell one chunk at a time does not publish
// again for every line.
func (r *Runner) publishInheritedFds(ctx context.Context) {
	if r.inheritedPublished {
		return
	}
	r.inheritedPublished = true
	for i, f := range r.InheritedFiles {
		fd := firstExtraFd + i
		if f == nil || fd > maxInheritedFd {
			continue
		}
		r.setFd(fd, f)
		// The gate is not asked, and the record is emitted anyway — see
		// ActionInherit, which says why the one without the other is the
		// honest answer here.
		r.emit(ctx, Event{Kind: EventAccess, Action: r.act(Action{
			Kind: ActionInherit,
			Path: inheritedPath(fd),
		})})
	}
}

// inheritedPath names a descriptor the way the operating system does, which
// is a real path on every platform this runs on rather than a label invented
// for the event: /dev/fd/3 is openable and is what the descriptor is called.
func inheritedPath(fd int) string { return "/dev/fd/" + itoa(fd) }

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp

import "os"

// The layout constants firstExtraFd and maxInheritedFd live in
// inheritedfds.go, because they describe both directions of the same table.

// childFiles rebuilds, for whatever this shell is about to hand the table to,
// the descriptors it holds beyond the three named streams.
//
// Two things read it, and they want the same answer. An external command is
// given it by number through os/exec's ExtraFiles; a *replacement* — `exec
// cmd`, the path that becomes the command rather than forking one — is given
// it by ReplaceProcess, which places each entry on its number before execve.
// The second is not a child and still inherits exactly what a child does:
// `exec 3>h; exec /bin/sh -c 'echo repl >&3'` writes in bash, dash and zsh
// just as the un-replaced form does, and the gaps and the exclusions below
// are the same there too.
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
//
// A descriptor this shell *inherited* and has since closed reaches as high as
// one it opened, and for the sake of the nil rather than the file. The
// original is still open in this process and is not close-on-exec — that is
// how it was recognized — so it would otherwise pass to a child through the
// kernel, behind the table's back: `exec 3<&-` then an external command found
// 3 still readable there, where all four shells find it closed. Reaching the
// number puts a nil at it, and a nil is a close.
func (r *Runner) childFiles() []*os.File {
	highest := 0
	for i, f := range r.InheritedFiles {
		if fd := firstExtraFd + i; f != nil && fd <= maxInheritedFd && fd > highest {
			highest = fd
		}
	}
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
	r.dropExecOpened(files)
	return files
}

// dropExecOpened takes back the descriptors `exec` opened, where the dialect
// says they are the shell's alone.
//
// A dialect question rather than a rule: four of the five hand them over, and
// one closes anything above 2 that `exec` opened before it runs anything —
// see ExecOpenedFdReachesACommand, which carries the measurements and the
// boundary. Nothing else in the table is touched, which is the whole of the
// difference: a descriptor the caller opened, and one this command redirected
// itself, are not `exec`'s and cross in every shell.
//
// The axis is asked only when the table actually holds one. An axis consulted
// on the common path would refuse every external command in a Runner that had
// not chosen a dialect, over a question that decides nothing for a script with
// no parked descriptors.
func (r *Runner) dropExecOpened(files []*os.File) {
	held := false
	for fd := range r.execFds {
		if i := fd - firstExtraFd; i >= 0 && i < len(files) && files[i] != nil {
			held = true
			break
		}
	}
	if !held {
		return
	}
	if r.ask(r.sem().ExecOpenedFdReachesACommand,
		"a descriptor `exec` parked reaching what the shell runs") {
		return
	}
	for fd := range r.execFds {
		if i := fd - firstExtraFd; i >= 0 && i < len(files) {
			// A nil rather than a gap: the number must be *closed* there, and
			// a nil is what says so on both routes out of this table.
			files[i] = nil
		}
	}
}

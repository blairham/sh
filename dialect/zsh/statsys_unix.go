// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build darwin || linux

package zsh

import (
	"os"
	"syscall"
)

// The platform half of `zstat`: the system call itself, and the fields it
// fills read back into types that do not vary.
//
// Split from statmodule.go the way errnotable.go is split, and for the same
// reason: the field widths are the operating system's, and a dialect that
// reached for them directly would have to be written twice. What is
// per-operating-system here is *only* the three timestamps, which two families
// of Unix spell differently in the same structure — see stattimes_darwin.go
// and stattimes_linux.go — so everything else is shared and cannot drift
// between them.

// statSupported is whether this platform's stat fields have been read. It
// gates the registration of `zstat` rather than its behavior, so a platform
// that is not here refuses at the `zmodload` line by the builtin's name
// instead of accepting the command and failing inside it.
const statSupported = true

// statPath is the stat of a name, following links or not.
func statPath(path string, follow bool) (statFields, error) {
	var st syscall.Stat_t
	var err error
	if follow {
		err = syscall.Stat(path, &st)
	} else {
		err = syscall.Lstat(path, &st)
	}
	if err != nil {
		return statFields{}, err
	}
	f := statFill(&st)
	if !follow {
		// The `link` element, which is the one of the fourteen the system call
		// does not produce: where the file points, when it is a link and the
		// question was about the link itself. Empty otherwise, which is why a
		// whole listing of an ordinary file ends in a bare name.
		f.link, _ = os.Readlink(path)
	}
	return f, nil
}

// statFd is the stat of an open descriptor, which is what `-f` asks for.
func statFd(fd int) (statFields, error) {
	var st syscall.Stat_t
	if err := syscall.Fstat(fd, &st); err != nil {
		return statFields{}, err
	}
	return statFill(&st), nil
}

// statFill reads one platform structure into the shared one.
func statFill(st *syscall.Stat_t) statFields {
	atime, mtime, ctime := statTimes(st)
	return statFields{
		device:  uint64(st.Dev),
		inode:   st.Ino,
		mode:    uint64(st.Mode),
		nlink:   uint64(st.Nlink),
		uid:     uint64(st.Uid),
		gid:     uint64(st.Gid),
		rdev:    uint64(st.Rdev),
		size:    st.Size,
		atime:   atime,
		mtime:   mtime,
		ctime:   ctime,
		blksize: int64(st.Blksize),
		blocks:  st.Blocks,
	}
}

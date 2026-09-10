// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package zsh

import (
	"errors"
	"os"
	"syscall"
)

const systemIOSupported = true

// sysopenFlag is one name from `-o`'s comma-separated list, and the open flag
// it stands for.
//
// The nine names are measured rather than derived from the platform's headers,
// which matters because the two sets are not the same one: `O_APPEND` exists
// on every system here and `-o append` is `unsupported option: append` — the
// flag is spelled `-a` — while `creat` and `create`, and `trunc` and
// `truncate`, are each two spellings of one flag. Measured 2026-09-10 against
// zsh 5.9.2 by writing every plausible name and recording which were taken:
// `noctify`, `noatime`, `direct`, `dsync`, `rsync`, `ndelay`, `rdonly`,
// `wronly` and `rdwr` are all refused there, so they are refused here.
//
// The comparison is case-insensitive and an `O_` prefix is allowed, both
// measured: `CLOEXEC`, `O_CLOEXEC` and `o_excl` are all taken. It is *not*
// prefix matching — `cloex`, `clo` and `exclusive` are each `unsupported
// option` — so this is a table of exact names rather than the shortening
// `zstat`'s `+element` does.
func sysopenFlag(name string) (flag int, cloexec, known bool) {
	switch name {
	case "cloexec":
		// Not an open flag here at all. Go opens every file close-on-exec
		// already, so passing O_CLOEXEC through would change nothing that a
		// child of this shell can see: the boundary is rebuilt by hand out of
		// the descriptor table — see interp's childFiles — and it is the
		// table that has to be told. That is what
		// [interp.Runner.KeepDescriptorFromChildren] is for, and it is why
		// the one name in this list that reads as a flag is answered as a
		// mark.
		return 0, true, true
	case "create", "creat":
		return os.O_CREATE, false, true
	case "excl":
		// **And O_CREAT with it**, which is measured rather than assumed and
		// is not what the name says: `sysopen -w -o excl` on a file that does
		// not exist *creates* it, and on one that does it is `file exists`.
		// O_EXCL alone is defined only alongside O_CREAT and this platform
		// simply ignores it without, so a shell that passed the one flag
		// through would open the existing file happily — a guard written as
		// `-o excl` to claim a lock file would claim one somebody else held.
		return os.O_CREATE | os.O_EXCL, false, true
	case "nofollow":
		return syscall.O_NOFOLLOW, false, true
	case "nonblock":
		return syscall.O_NONBLOCK, false, true
	case "sync":
		return os.O_SYNC, false, true
	case "trunc", "truncate":
		return os.O_TRUNC, false, true
	}
	return 0, false, false
}

// sysOpenFile is the open itself, and it exists for one flag.
//
// `-o nonblock` asks that the *open* not wait for the other end of a pipe,
// which is what it does in zsh and is the only observable thing it does
// there: a `sysread` on the descriptor afterwards waits for data exactly as
// it does on a descriptor opened without it. Measured — a process
// substitution read through a nonblocking descriptor gives its bytes in zsh,
// and a fifo with no writer at all is opened at once by both.
//
// Leaving the flag on the file description gives neither half of that. The
// runtime would carry the descriptor non-blocking and a read of a pipe
// nothing has written to yet fails rather than waits, so `-o nonblock` would
// have turned every subsequent read into a race. Clearing it after the open
// is the same thing interp's own fifo ends do, and openFifoWriteEnd states
// the reason in those words.
//
// So the flag is neither passed through nor ignored: it is spent on the open
// and taken off. A caller who wrote it gets what they asked for — an open
// that does not hang on a peer — and a shell that had kept it would have
// given them something else.
func sysOpenFile(path string, flags int, perm os.FileMode) (*os.File, error) {
	if flags&syscall.O_NONBLOCK == 0 {
		return os.OpenFile(path, flags, perm)
	}
	var fd int
	var err error
	for {
		fd, err = syscall.Open(path, flags|syscall.O_CLOEXEC, uint32(perm.Perm()))
		if !errors.Is(err, syscall.EINTR) {
			break
		}
	}
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	if err := syscall.SetNonblock(fd, false); err != nil {
		_ = syscall.Close(fd)
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(fd), path), nil
}

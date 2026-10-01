// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package zsh

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// systemLockSupported says whether `zsystem supports flock` may answer 0 on
// this build. It is what makes that answer a fact about the platform rather
// than a promise this package makes on the platform's behalf.
const systemLockSupported = true

// systemLockOpen opens the file a lock is to be taken on.
//
// The direction is the lock's: a read lock needs a descriptor open for
// reading and a write lock one open for writing, which is what makes
// `zsystem flock -r` usable on a file nobody may write and `zsystem flock`
// on the same file `failed to open … for writing: permission denied`. Both
// measured.
//
// Nothing is created and nothing is truncated. A lock file that does not
// exist is `failed to open … : no such file or directory`, measured, and it
// is the answer a script wants: a caller that meant to create it writes
// `sysopen -o creat` or a redirection first, and a lock builtin that created
// what it was pointed at would make a typo look like a successful claim.
func systemLockOpen(path string, reading bool) (*os.File, error) {
	flags := os.O_WRONLY
	if reading {
		flags = os.O_RDONLY
	}
	return os.OpenFile(path, flags, 0o666)
}

// systemLockTake asks for the lock, and is the whole of the difference
// between the three waits.
//
// block is a wait with no end, which is the system's own blocking form and
// costs nothing while it waits. A wait with a deadline cannot use that form —
// the call has no timeout in it and cannot be taken back once made — so it is
// the asking form, repeated until the deadline. wait of zero with block false
// is one ask and no repeat, which is what `-t 0` means.
//
// The error is returned alongside the verdict rather than folded into it,
// because the caller says two different things about a failure depending on
// whether it waited: see zsystemLock.
func systemLockTake(fd int, reading, block bool, wait, interval time.Duration, claim func() bool, waiting func()) (bool, error) {
	kind := int16(syscall.F_WRLCK)
	if reading {
		kind = syscall.F_RDLCK
	}
	lock := syscall.Flock_t{Type: kind, Whence: 0, Start: 0, Len: 0}
	if interval <= 0 {
		interval = time.Millisecond
	}
	deadline := time.Now().Add(wait)
	// The first ask never blocks, even for a wait with no end: whether this
	// is going to wait at all is something the caller is told before it
	// does. See waiting's caller.
	cmd, waited := syscall.F_SETLK, false
	for {
		err := syscall.FcntlFlock(uintptr(fd), cmd, &lock)
		switch {
		case err == nil:
			if claim() {
				return true, nil
			}
			// The system said yes because the holder is this process: a
			// body of this shell that a real shell would have forked holds
			// it. Answered the way the system answers a holder elsewhere —
			// a refusal for an ask, and another try for a wait. See
			// interp.Process.
			err = syscall.EAGAIN
		case errors.Is(err, syscall.EINTR):
			// A signal arrived while waiting. The request is unchanged, so
			// it is made again — the alternative is a lock builtin that
			// fails whenever anything at all happens to the shell.
			continue
		case !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EACCES):
			// Not "somebody else has it" — a descriptor open the wrong way
			// for the lock asked for, a file system with no locking. Waiting
			// would not change it.
			return false, err
		}
		left := time.Until(deadline)
		if !block && left <= 0 {
			return false, err
		}
		if !waited {
			waited = true
			waiting()
		}
		if block {
			// Somebody holds it, so from here the system's own blocking
			// form does the waiting — which returns at once for a holder
			// in this process, and that holder is waited out by the poll.
			cmd = syscall.F_SETLKW
			time.Sleep(interval)
			continue
		}
		time.Sleep(min(interval, left))
	}
}

// systemLockRelease gives up whatever lock this descriptor holds.
func systemLockRelease(fd int) {
	lock := syscall.Flock_t{Type: syscall.F_UNLCK, Whence: 0, Start: 0, Len: 0}
	for {
		if err := syscall.FcntlFlock(uintptr(fd), syscall.F_SETLK, &lock); !errors.Is(err, syscall.EINTR) {
			return
		}
	}
}

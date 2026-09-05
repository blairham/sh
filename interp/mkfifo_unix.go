// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// mkfifo makes a named pipe.
//
// 0600: what passes through it is this shell's data, and a pipe anyone on the
// machine can open is a pipe anyone can read.
func mkfifo(path string) error { return syscall.Mkfifo(path, 0o600) }

// The two ends of a substitution's pipe are not the same problem, and the
// asymmetry below is the kernel's rather than a choice.
//
// Opening a FIFO waits for the other side: for writing until there is a
// reader, for reading until there is a writer. A shell handing a path to a
// command cannot know whether the command will ever open it — `echo <(true)`
// is a path printed and nothing more, and is an error in no shell — so every
// one of these waits is a wait that may never end. It used to be exactly
// that: an endless open on a goroutine of its own, which a one-shot binary
// takes with it on exit and a program that embeds this package does not. One
// parked goroutine and one blocked OS thread per unopened substitution, for
// the life of the process.
//
// The reading side does not have to wait at all. The writing side does, and
// cannot be talked out of it: a reader arriving later needs a writer to open
// against, so a shell that opened its own writing end early and closed it
// again would leave that reader waiting forever — a worse failure than the
// one being fixed, since it is the *command* that hangs. What is fixed
// instead is the endlessness.

// openFifoWriteEnd opens the shell's writing end of a substitution's pipe,
// once something has opened the reading end, and gives up when nothing will.
//
// O_NONBLOCK turns the wait into a question: with no reader the open fails
// with ENXIO instead of blocking, so the waiting becomes ours to bound. What
// bounds it is the pipe itself. The name is removed as soon as the command
// that was given it is done — that is removeProcSubs, which already ran on
// every path — so an open that reports ENOENT is an open that nobody is ever
// going to answer. The give-up condition is a fact about the file system
// rather than a second channel to keep in step with the first.
//
// It cannot give up on a reader that is really there. A successful
// nonblocking open means a reader held the pipe open at that instant, and a
// reader that is going to *read* cannot finish before the shell has written,
// because what it is waiting for is this end closing. Only a command that
// opens the path and closes it again without reading can be missed, and such
// a command gets nothing either way.
//
// The flag is cleared once the open succeeds, because it belongs to the open
// file description and would otherwise travel into the command on the far
// side of the substitution, where a write longer than a pipe buffer would
// fail with EAGAIN rather than wait.
func openFifoWriteEnd(path string) (*os.File, error) {
	// Short enough that the usual case — a command that opens the path as
	// soon as it starts — is not made to wait for the poll, long enough that
	// a command which opens it late is not spun on.
	const (
		firstWait = 100 * time.Microsecond
		lastWait  = 5 * time.Millisecond
	)
	for wait := firstWait; ; {
		fd, err := syscall.Open(path, syscall.O_WRONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
		switch {
		case err == nil:
			if err := syscall.SetNonblock(fd, false); err != nil {
				_ = syscall.Close(fd)
				return nil, err
			}
			return os.NewFile(uintptr(fd), path), nil
		case errors.Is(err, syscall.EINTR):
			continue
		case !errors.Is(err, syscall.ENXIO):
			// ENOENT among them: the pipe has been taken away, so there is
			// no reader coming.
			return nil, err
		}
		time.Sleep(wait)
		if wait *= 2; wait > lastWait {
			wait = lastWait
		}
	}
}

// openFifoReadEnd opens the shell's reading end of a substitution's pipe,
// along with a placeholder that keeps the pipe usable while the command that
// was given the path decides what to do with it.
//
// Neither open here can block, and both need saying. O_RDONLY|O_NONBLOCK on a
// FIFO succeeds whether or not a writer has arrived, which is what lets the
// substituted command start at once instead of waiting to be written to.
// O_RDWR|O_NONBLOCK never waits either, and is the placeholder: it holds the
// pipe open in *both* directions, and each direction answers a way the
// arrangement would otherwise fail.
//
// As a writer, it is why the substituted command does not see an immediate
// end-of-file and exit before the caller has written anything: `echo hi >
// >(cat)` would print nothing at all.
//
// As a reader, it is why the caller's own open succeeds even when the
// substituted command has already finished: `echo hi > >(true)` closes the
// shell's reading end almost immediately, and opening a FIFO for writing with
// no reader waits — so the caller's redirection, not the shell's goroutine,
// would be the thing that hung.
//
// The placeholder is closed when the command that named the path is done with
// it, which is the end-of-file the substituted command has been waiting for.
func openFifoReadEnd(path string) (end, hold *os.File, err error) {
	hfd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	efd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		_ = syscall.Close(hfd)
		return nil, nil, err
	}
	// Cleared for the same reason as on the writing end: the substituted
	// command reads through this descriptor, and a nonblocking read of a
	// pipe nobody has written to yet fails rather than waits.
	if err := syscall.SetNonblock(efd, false); err != nil {
		_ = syscall.Close(efd)
		_ = syscall.Close(hfd)
		return nil, nil, err
	}
	return os.NewFile(uintptr(efd), path), os.NewFile(uintptr(hfd), path), nil
}

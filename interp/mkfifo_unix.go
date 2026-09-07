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

// nudgeFifoEOF repeats the last-writer close until there is no reader left to
// tell about it, or the pipe is gone.
//
// # Why a close has to be repeated
//
// A reader learns that there is no more input when the pipe's last writer
// closes. On this platform that telling can be **lost**, and the state it is
// lost from is one nothing in the shell can distinguish from a healthy one.
//
// Measured 2026-09-07, macOS 26.5.2 on arm64, `GOMAXPROCS=1`. Running
// `cat <(echo sub; echo noise >&2)` in-process with an eight-second bound
// stopped in every one of eight runs of 1,500 to 2,000 rounds — about one
// round in six hundred. At the stop:
//
//   - the shell is in `wait4` on `cat`, and both of os/exec's drain
//     goroutines are in `poll.Read`;
//   - the goroutine that produced the substitution has **finished**, so the
//     write end was opened, written and closed;
//   - `lsof` of the shell process shows **no descriptor on the pipe at all**,
//     so the close really happened;
//   - `cat` holds it as `3r FIFO`, offset `0t4` — it took the four bytes —
//     and is asleep in `read`;
//   - a **new** reader opening the same pipe and reading gets `0, nil` — end
//     of file — at once. The pipe knows it has no writers. Only the reader
//     already parked in it was never told;
//   - opening the write end again and closing it releases `cat` in about
//     five milliseconds.
//
// # What loses it
//
// The race is between the writer's **close** and the reader's **open**, not
// between the close and a read. `cat path` opens with a plain `O_RDONLY`,
// which on a FIFO waits for a writer; the shell's poll for the write end
// succeeds as soon as that reader is waiting. If the shell then closes before
// the reader's open has returned, the reader can come out of `open` into a
// pipe whose end-of-file has already been and gone.
//
// That is reproducible with no interpreter in sight, which #1079 could not
// manage and this is the correction to it. A standalone program that mkfifos,
// starts `/bin/cat` on the path and opens the write end exactly as
// openFifoWriteEnd does stops **14 times in 1,500 rounds** at `GOMAXPROCS=1`
// when the writer closes without writing — and **0 times in 1,500** when it
// writes four bytes first, which is what #1079's standalone did and why it
// saw nothing. Writing widens the window between the poll succeeding and the
// close by enough to usually win it; it does not close it, which is why the
// interpreter loses with a write too. With this repeated close the same
// standalone is 0 in 3,000.
//
// # Why repeating the close is the fix
//
// Opening for writing and closing again is exactly the transition that was
// lost, and it is the only one that delivers an end-of-file: there is no
// other way to tell a pipe's reader that its input has ended. Holding
// something open instead — the placeholder trick `>(cmd)` uses in the other
// direction — cannot work here, because for `<(cmd)` the shell is the writer
// and anything holding the write end open is the very thing keeping the
// end-of-file away.
//
// Each round costs one open and one close, and there are two ways out and no
// third: ENXIO says no reader is left to tell, and anything else — ENOENT
// above all, which is removeProcSubs having taken the pipe away at the end of
// the command that named it — says there is no pipe to tell through. So this
// cannot outlive the command, and in the ordinary case it ends on the first
// or second round, as soon as the reader has taken its end-of-file and gone.
//
// It is not gated on the platform. A lost wakeup is not a thing a program can
// ask about, and a shell that only worked on the kernels somebody had
// measured would be worse than one that repeats a close nobody needed: where
// the first close was heard, the reader has already gone and the first open
// here answers ENXIO.
//
// The opens are the shell's own scaffolding on a path the script never wrote,
// exactly as the first open was, so the gate is not asked and no event is
// emitted: this is one close, repeated, rather than a new access. See
// Runner.ownPipe for the argument in full.
func nudgeFifoEOF(path string) {
	const (
		firstWait = 100 * time.Microsecond
		lastWait  = 20 * time.Millisecond
	)
	for wait := firstWait; ; {
		fd, err := syscall.Open(path, syscall.O_WRONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
		if err != nil {
			if errors.Is(err, syscall.EINTR) {
				continue
			}
			// ENXIO: nobody is reading, so there is nobody to tell. Anything
			// else — ENOENT most of all — means the pipe is not there to be
			// told through.
			return
		}
		_ = syscall.Close(fd)
		time.Sleep(wait)
		if wait *= 2; wait > lastWait {
			wait = lastWait
		}
	}
}

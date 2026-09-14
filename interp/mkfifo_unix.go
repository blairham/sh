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
//
// What both directions turn out to need is the same thing, and it took until
// #2733 to see that it was: an end of the shell's own, opposite the one the
// pipe is for, held for as long as the command that named the path is using
// it. Each direction's is below, and neither is a descriptor anything reads
// or writes — they exist so that the pipe is still the same pipe when the two
// real ends finally meet in it.

// openFifoWriteEnd opens the shell's writing end of a substitution's pipe,
// once something has opened the reading end, and gives up when nothing will.
// It answers with a reading end of the shell's own beside it, which is what
// keeps what the body writes from being thrown away.
//
// O_NONBLOCK turns the wait into a question: with no reader the open fails
// with ENXIO instead of blocking, so the waiting becomes ours to bound. What
// bounds it is the pipe itself. The name is removed as soon as the command
// that was given it is done — that is removeProcSubs, which already ran on
// every path — so an open that reports ENOENT is an open that nobody is ever
// going to answer. The give-up condition is a fact about the file system
// rather than a second channel to keep in step with the first.
//
// It cannot give up on a reader that is really there: a successful
// nonblocking open means a reader held the pipe open at that instant, and
// only a command that opens the path and closes it again without reading can
// be missed — such a command gets nothing either way.
//
// The flag is cleared once the open succeeds, because it belongs to the open
// file description and would otherwise travel into the command on the far
// side of the substitution, where a write longer than a pipe buffer would
// fail with EAGAIN rather than wait. It is left set on the placeholder, which
// nothing ever reads through.
//
// # Why the shell holds a reading end too
//
// This function used to claim more than the open can support: that a reader
// which is going to *read* cannot finish before the shell has written,
// because what it is waiting for is this end closing. That is false on this
// platform, and #2733 is the measurement — 40,000 rounds of each of two
// spellings, eight shells at once, about one round in four thousand losing
// the body's first chunk, or all of it, or never finishing at all.
//
// A FIFO's pipe exists only while somebody holds it open; when the last
// reader and the last writer have gone, the buffer goes with them and the
// next open makes a new one. The open above succeeds against a reader that
// is still *inside* `open(2)` — counted enough to answer ENXIO, not yet
// attached to anything — so a shell that opens, writes and closes inside
// that window has run a whole pipe's life cycle beside a reader that was
// attached to none of it. All three shapes in the issue are that: the write
// refused with a spurious EPIPE, the writes accepted into a pipe that is
// then discarded, and the reader left parked in an `open` nothing will
// complete.
//
// A reading end of the shell's own, taken here — before the body has written
// a byte — and released when the command that named the path is done with
// it, is what makes the pipe outlast that window: 0 lost in 40,000 where the
// tree without it loses 25. It is also the arrangement `>(cmd)` has had
// since the beginning, for the mirror-image reason; see openFifoReadEnd.
//
// O_RDONLY and not the O_RDWR that direction uses, and the difference is the
// whole point of each. That placeholder has to be a *writer*, to keep an
// end-of-file away from a body that would otherwise see one immediately.
// This one must not be one: the end-of-file the command is reading until is
// this shell's writing end closing, so the count of writers still has to
// fall to zero when the body is done. A reader is all that is added, and all
// that is added is that the pipe is still there to deliver through.
//
// Nothing reads from it. A descriptor that read would be taking the bytes the
// command was given the path for.
//
// # And why it does not replace the nudge
//
// It was expected to, and the measurement said otherwise — which is the one
// thing here worth remembering, because the two look like the same fix for
// the same race and are not. nudgeFifoEOF below repeats the last-writer
// close for a reader that never heard the first one. Take it away and leave
// only the placeholder, and the byte loss above goes to **0 in 40,000**
// while #1079's own stress case stops again at round 991 of 2000: `cat
// <(echo sub; echo noise >&2)` parked forever.
//
// The two failures are two states, and each answer reaches only its own. The
// placeholder is a *reader*, and it keeps the pipe — and so the bytes in it
// — from being torn down under a command that is still arriving. A command
// parked in `open(O_RDONLY)` is waiting for a **writer**, which no reader of
// ours can be without also holding away the end-of-file the command is
// there for. Only opening the write end again is that transition.
//
// So they compose, and what composing costs is the nudge's first answer.
// ENXIO — no reader left to tell — is unreachable while the placeholder is
// up, since the placeholder is a reader; what ends the loop instead is the
// pipe's name going away with the command that named it, which is the same
// removeProcSubs that releases the placeholder, or the deadline. See
// nudgeFifoEOF for why that is a bound rather than a spin.
func openFifoWriteEnd(path string) (end, hold *os.File, err error) {
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
				return nil, nil, err
			}
			// Before anything is written through the descriptor above, which
			// is the only ordering that matters here.
			hfd, herr := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
			if herr != nil {
				_ = syscall.Close(fd)
				return nil, nil, herr
			}
			return os.NewFile(uintptr(fd), path), os.NewFile(uintptr(hfd), path), nil
		case errors.Is(err, syscall.EINTR):
			continue
		case !errors.Is(err, syscall.ENXIO):
			// ENOENT among them: the pipe has been taken away, so there is
			// no reader coming.
			return nil, nil, err
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
// tell about it, the pipe is gone, or it has been repeated for longer than a
// lost wakeup could need.
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
// # Why repeating the close is the fix, and why the placeholder is not
//
// Opening for writing and closing again is exactly the transition that was
// lost, and it is the only one that delivers an end-of-file: there is no
// other way to tell a pipe's reader that its input has ended. The reading
// end openFifoWriteEnd holds is not a substitute and was tried as one —
// **0 byte losses in 40,000 and this case parked at round 991 of 2,000** —
// because a reader is not a writer and what the parked command is waiting
// for is a writer. A descriptor of ours that *were* one would hold away the
// very end-of-file this delivers.
//
// Each round costs one open and one close, and there are three ways out.
// ENXIO says no reader is left to tell; anything else — ENOENT among them —
// says there is no pipe to tell through; and a deadline says the transition
// has been repeated for longer than a lost wakeup could plausibly need.
//
// **Which of the three ends it moved when the placeholder arrived**, and the
// answer is worth stating because the first one used to carry the ordinary
// case. ENXIO cannot arrive while the placeholder is up, since the
// placeholder is a reader; what ends an ordinary round now is ENOENT, from
// the same removeProcSubs that releases the placeholder at the end of the
// command that named the path — so the loop still stops when the
// substitution is over rather than running out its clock. A pipe whose name
// this shell is keeping open (#1750) has neither, and the deadline is what
// it ends on.
//
// It is a deadline rather than a count because what it bounds is time: the
// race it covers is between two system calls on two threads, and a hundred
// milliseconds is several orders of magnitude more than that window, while
// the round count that spans it changes with the backoff above. Before it
// went in, the shape with a kept name went round every twenty milliseconds
// for the life of the shell, per substitution (#1907).
//
// It is not gated on the platform. A lost wakeup is not a thing a program can
// ask about, and a shell that only worked on the kernels somebody had
// measured would be worse than one that repeats a close nobody needed.
//
// The opens are the shell's own scaffolding on a path the script never wrote,
// exactly as the first open was, so the gate is not asked and no event is
// emitted: this is one close, repeated, rather than a new access. See
// Runner.ownPipe for the argument in full.
func nudgeFifoEOF(path string) {
	const (
		firstWait = 100 * time.Microsecond
		lastWait  = 20 * time.Millisecond
		giveUp    = 100 * time.Millisecond
	)
	deadline := time.Now().Add(giveUp)
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
		if time.Now().After(deadline) {
			// The transition has been delivered for long enough. A reader
			// that is still there is one holding the pipe for its own
			// reasons rather than one that missed an end-of-file.
			return
		}
		time.Sleep(wait)
		if wait *= 2; wait > lastWait {
			wait = lastWait
		}
	}
}

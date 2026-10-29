// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"io"
	"time"

	"github.com/blairham/sh/internal/fdset"
)

// polledTimedSource is the byte source of a `read -t` on a stream that is a
// descriptor — standard input, a pipe, a `-u` number, the terminal `read -k`
// holds — and false where the stream is not one that can be polled. Those
// keep timedByteSource, whose reader goroutine is the only way to put a
// deadline on an io.Reader.
func polledTimedSource(
	ctx context.Context,
	in io.Reader,
	timeout time.Duration,
	bounded bool,
) (next func() (byte, int), can bool) {
	if timeout <= 0 {
		return nil, false
	}
	f, ok := in.(interface{ Fd() uintptr })
	if !ok {
		return nil, false
	}
	fd := int(f.Fd())
	if fd < 0 {
		return nil, false
	}
	return pollingDeadlineSource(ctx, in, fd, timeNow().Add(timeout), bounded)
}

// timeNow is time.Now, named so the deadline has one source.
func timeNow() time.Time { return time.Now() }

// pollingDeadlineSource reads a descriptor a byte at a time, waiting for each byte
// to be *there* before asking for it, and never leaving a read in flight.
//
// **This is not an optimization; it is the difference between a timeout and
// one that eats the next reader's input.** timedByteSource reads on a
// goroutine, because an io.Reader cannot be told to stop waiting, so a read
// still in flight when the deadline passes is abandoned — and when its byte
// arrives, that read takes it and nobody ever sees it. It was first found on
// the terminal: zsh's history-search plugin ends with `read -k -t 1`, and the
// next key the person pressed lost its escape byte to the abandoned read and
// typed `[A` into the line. It is the same on a pipe. Measured 2026-10-29,
// with a byte that arrives only after the deadline:
//
//	{ sleep 0.5; printf 'xy\n'; } |
//	  sh -c 'v=old; read -t 0.01 -r v; echo "st=$? [$v]"; read -r w; echo "[$w]"'
//
// bash 5.3.20, bash 3.2.57, ksh93u+, zsh 5.9.2 and BusyBox ash 1.37.0 all
// hand `xy` to the second read. The goroutine handed it `y`.
//
// A descriptor can be asked whether a read would block, which an io.Reader
// cannot — so this waits for readability and reads only what is already
// there. Nothing is ever in flight, so nothing is ever abandoned.
//
// **The first byte is always looked for, however late the look.** A deadline
// bounds a *wait*, and a byte already in the stream makes nobody wait: bash
// takes it at every deadline down to 100 µs (ten runs of ten), and only below
// its own overhead does the look become a race. So the first look waits for
// whatever is left of the deadline, which may be nothing. That is also what
// makes the answer deterministic: the only thing that can make the look late
// is this shell's own scheduling, which nothing outside can see. Before this,
// a 1 ms deadline over a byte that was already there lost the byte on a
// loaded machine about one time in a hundred (#7021).
//
// **After the first byte, a deadline that bounds the whole read stops it**,
// without another look, even if more bytes are waiting. A stream that never
// pauses is the case that says so: 262144 bytes with no newline under `-t
// 0.05` is 142 with 53051 of them in bash and 1 with the name untouched in
// ksh93 and ash. Looking again after the deadline would have read the lot.
// Where the deadline bounds only the wait for the first byte, the rest of the
// read is made without one — ReadTimeoutBoundsReadability decides which this
// is, and the caller hands the answer in.
//
// A timeout written as zero never reaches here. It is a question about the
// stream rather than a deadline, and pollingByteSource answers it — one
// dialect keeps taking whatever is waiting there, which is exactly what a
// deadline that has passed must not do.
//
// asked is false where the question cannot be put to this descriptor at all,
// and the caller then falls back to the goroutine: a `read -t` that reported a
// timeout it never waited for would be worse than one that loses a byte.
func pollingDeadlineSource(
	ctx context.Context,
	in io.Reader,
	fd int,
	deadline time.Time,
	bounded bool,
) (next func() (byte, int), asked bool) {
	if _, can := fdset.ReadableWithin(fd, 0); !can {
		return nil, false
	}
	var ch [1]byte
	first := true
	return func() (byte, int) {
		if first || bounded {
			if !first && !timeNow().Before(deadline) {
				return 0, evTimeout
			}
			ready, can := awaitReadable(ctx, fd, deadline)
			if !can {
				return 0, evEOF
			}
			if !ready {
				return 0, evTimeout
			}
		}
		first = false
		n, err := in.Read(ch[:])
		if n == 0 || err != nil {
			return 0, evEOF
		}
		return ch[0], evByte
	}, true
}

// canceledReadNoticedWithin is how long a timed read waiting on a descriptor
// can go without noticing that its context has ended.
const canceledReadNoticedWithin = 50 * time.Millisecond

// awaitReadable waits until fd has something to read, the deadline passes, or
// ctx ends, and looks at least once whichever comes first.
//
// The wait is cut into slices where ctx can end, because the goroutine this
// source replaces gave up the moment its context did, and an embedder that
// cancels a running line — termhost's Kill does exactly that — expects a timed
// read inside it to stop rather than to run out its deadline. A context that
// can never end is waited on in one call.
func awaitReadable(ctx context.Context, fd int, deadline time.Time) (ready, can bool) {
	for {
		left := time.Until(deadline)
		if ctx.Done() != nil && left > canceledReadNoticedWithin {
			left = canceledReadNoticedWithin
		}
		ready, can = fdset.ReadableWithin(fd, left)
		if ready || !can {
			return ready, can
		}
		if ctx.Err() != nil || !timeNow().Before(deadline) {
			return false, true
		}
	}
}

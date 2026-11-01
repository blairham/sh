// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"io"
	"os"
	"reflect"
	"time"

	"github.com/blairham/sh/internal/tty"
)

// Two things ksh93 does with a `read -n` that the rest of the panel does not
// (#7179), each an axis of its own because each is keyed on something the
// other is not:
//
//   - Semantics.ReadCountOfAPipeExecOpenedFindsItsEnd: a `-n` read of a
//     `-u` number `exec` put there, holding a pipe, a fifo or any other
//     stream that is neither a regular file nor a terminal, finds the stream
//     at its end as soon as there is anything to read, and takes nothing
//     from it. Which numbers `exec` put there is Runner.execFds, on the
//     terms ExecOpenedFdReachesACommand already keeps it.
//   - Semantics.ReadCountAtTheEndSticksTheDescriptor: a `-n` read that finds
//     the end of its input before its first character leaves the number it
//     read stuck, and every later `read` of that number answers 0 with its
//     names empty and reads nothing, until the number is opened again.
//
// The first is what makes the second reachable on a pipe that still has a
// line in it, which is the shape the issue was found in.

// countedReadFindsItsEnd reports whether in is a stream the first of those
// takes for at its end: a descriptor that is neither a regular file nor a
// terminal. Measured 2026-10-31 on ksh93u+: a pipe, a fifo, a process
// substitution and /dev/zero are; a regular file, a here-document (a
// temporary file there) and a terminal are not.
//
// Only an *os.File is asked. Anything else is a reader this shell holds in
// memory — a here-document it could not spool, an embedder's buffer — and is
// read as a file is.
func countedReadFindsItsEnd(in io.Reader) (*os.File, bool) {
	f, ok := in.(*os.File)
	if !ok {
		return nil, false
	}
	st, err := f.Stat()
	if err != nil || st.Mode().IsRegular() || tty.IsTerminal(f) {
		return nil, false
	}
	return f, true
}

// streamAtItsEnd is a stream read as though it had ended, once it has
// something to read or has really ended: ksh93's `read -n 4 -u3` with nothing
// yet in the pipe waits for the writer and then answers 1, and the bytes the
// writer sent are still there for `cat <&3`.
//
// It keeps the descriptor's number, so a timed read and a read a trapped
// signal can give up wait on it the way they wait on the stream itself, and
// only then ask it for a byte.
type streamAtItsEnd struct {
	ctx context.Context
	f   *os.File
}

func (s streamAtItsEnd) Read([]byte) (int, error) {
	if fd := int(s.f.Fd()); fd >= 0 {
		awaitReadable(s.ctx, fd, time.Time{})
	}
	return 0, io.EOF
}

// Fd is the stream's own number, for the waits that poll it.
func (s streamAtItsEnd) Fd() uintptr { return s.f.Fd() }

// stickRead records that descriptor fd, which is stream, has had a `-n` read
// find its end, in the dialect that keeps that. A stream that cannot be
// compared is not recorded, and reads on.
func (r *Runner) stickRead(fd int, stream io.Reader) {
	if stream == nil || !reflect.TypeOf(stream).Comparable() {
		return
	}
	if r.stuckReads == nil {
		r.stuckReads = map[int]io.Reader{}
	}
	r.stuckReads[fd] = stream
}

// readIsStuck reports whether a read of descriptor fd, which is stream, is one
// stickRead recorded: the same number, still holding the same stream.
//
// The number is the key and the stream is the check. A copy of a stuck
// number reads on — `exec 4<&3` after 3 sticks, or `exec 3<&0` after 0 does —
// and so does the number once it is opened again, which setFd sees for every
// number past 2 and the stream itself tells for standard input. A command's
// own `read -n 4 l <empty` sticks the file it opened and not the standard
// input that comes back after it.
func (r *Runner) readIsStuck(fd int, stream io.Reader) bool {
	held, ok := r.stuckReads[fd]
	return ok && held == stream
}

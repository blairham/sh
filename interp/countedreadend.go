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
//
// And one thing it does with a `-N` read (#7177):
//
//   - Semantics.ReadExactCountGivenUpKeepsWhatItTook: a `-N` read a trapped
//     signal gives up keeps the bytes it took, and the next `read` of the
//     same number takes them first.
//
// The stuck number and the kept bytes are both a mark on the number, made by
// one read and heeded by the next while the number holds the same stream;
// readMark is that mark.

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

// readMark is what one number's stream carries from one `read` to the next,
// in the dialect that carries anything: that a `-n` read found it at its end,
// or the bytes a `-N` read a trapped signal gave up had taken. stream is the
// stream the number held when the mark was made, and a read of the number
// heeds the mark only while it still holds that stream.
type readMark struct {
	stream io.Reader
	stuck  bool
	kept   string
}

// markRead puts m on descriptor fd, which is stream. A stream that cannot be
// compared is not marked, and reads on as though nothing had happened.
func (r *Runner) markRead(fd int, stream io.Reader, m readMark) {
	if stream == nil || !reflect.TypeOf(stream).Comparable() {
		return
	}
	if r.readMarks == nil {
		r.readMarks = map[int]readMark{}
	}
	m.stream = stream
	r.readMarks[fd] = m
}

// stickRead records that descriptor fd, which is stream, has had a `-n` read
// find its end, in the dialect that keeps that.
func (r *Runner) stickRead(fd int, stream io.Reader) {
	r.markRead(fd, stream, readMark{stuck: true})
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
	m, ok := r.readMarks[fd]
	return ok && m.stuck && m.stream == stream
}

// forgetReadMarkOfAnotherStdin takes the mark off standard input once a
// command's redirections are undone and stdin, what it was before them, is
// standard input again, where the mark was left on a stream other than that
// one: the command's own, which goes with the command. Its number is 0 either
// way, and a copy — `read l <&3` — is the same stream as the number it copies,
// so neither the number nor the stream tells the two apart afterwards.
//
// Measured 2026-10-31 on ksh93u+: `exec 3<>fifo; read -r -N 4 l <&3` given
// up by INT with `ab` taken leaves the next `read -r m <&3` reading `cd`, and
// `: >f; exec 3<f; read -n 4 l <&3; echo abc >>f; read m <&3` reads `abc` at
// 0 (#7177).
func (r *Runner) forgetReadMarkOfAnotherStdin(stdin io.Reader) {
	if m, ok := r.readMarks[0]; ok && m.stream != stdin {
		delete(r.readMarks, 0)
	}
}

// keepRead holds text, what a `-N` read a trapped signal gave up had taken,
// for the next `read` of descriptor fd while it holds stream, in the dialect
// that keeps it (#7177). Nothing outside the shell sees it: the bytes are off
// the stream already, so a command run after the read reads what came after
// them. See Semantics.ReadExactCountGivenUpKeepsWhatItTook.
func (r *Runner) keepRead(fd int, stream io.Reader, text string) {
	if text != "" {
		r.markRead(fd, stream, readMark{kept: text})
	}
}

// keptRead is what keepRead holds for a read of descriptor fd, which is
// stream: on the terms readIsStuck keeps, the number and then the stream, so
// a copy of the number, the source it was copied from and the number opened
// again read the stream itself. The caller hands back what it did not use.
func (r *Runner) keptRead(fd int, stream io.Reader) string {
	m, ok := r.readMarks[fd]
	if !ok || m.kept == "" || m.stream != stream {
		return ""
	}
	delete(r.readMarks, fd)
	return m.kept
}

// keptFirst is a byte source that hands out kept before it asks next, the
// source of a `read` whose number holds what a given-up `-N` read took. The
// kept bytes are there to be read and are handed out without a wait, the way
// a byte already in the stream would be.
type keptFirst struct {
	kept string
	at   int
	next func() (byte, int)
}

func (k *keptFirst) read() (byte, int) {
	if k.at < len(k.kept) {
		c := k.kept[k.at]
		k.at++
		return c, evByte
	}
	return k.next()
}

// rest is what the read did not take of kept, still the number's.
func (k *keptFirst) rest() string { return k.kept[k.at:] }

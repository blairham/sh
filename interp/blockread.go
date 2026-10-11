// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"io"
	"os"
)

// The line readers — `read`, `mapfile`, `select` — take their input a byte at
// a time from the stream's point of view, because what a reader leaves behind
// is the next reader's: `{ read -r l; cat; } < f` hands `cat` the file from
// the byte after the first newline, in every shell in the panel and on every
// kind of input. Measured 2026-10-11 on bash 5.3.20, zsh 5.9.2, ksh93u+ and
// dash, over a regular file and over a pipe alike; `read -n 2` and `read -d e`
// leave the offset just past what they took in the three that have the
// letters, and `mapfile -n 1` leaves the rest of the stream on a pipe as well
// as on a file.
//
// Asking the kernel for one byte per call is the obvious way to get there and
// it costs a system call per byte: #6355 measured 3.9 million of them for a
// 100,000-line file, 0.89 s of system time where bash spends 0.08 s. On a
// regular file the same offset is reachable without that: read a block, hand
// the bytes out one at a time, and when the reader is done move the offset
// back over whatever it did not take. Nothing outside the shell can tell the
// two apart — the offset afterwards is the same byte, and the bytes the reader
// saw are the same bytes — so this is not a dialect question, and every
// dialect takes the fast path whether or not the shell it claims to be reads
// a byte per call on a file.
//
// A pipe, a terminal or anything else that cannot be put back keeps the byte
// at a time: what is read from one is gone, and a reader that took more than
// it used would take it from whoever reads next.

// A seekableFile is a stream that may be read ahead and put back: one that
// can say what it is, and move its offset. *os.File is the one that matters.
type seekableFile interface {
	io.Reader
	io.Seeker
	Stat() (os.FileInfo, error)
}

// rewindableFile reports whether in is a regular file, which is the one kind of
// stream every byte of whose offset is the reader's to set. A character
// device, a FIFO or a socket may answer a seek and still not give back what
// was read, so the question is the file's kind and not whether Seek fails.
func rewindableFile(in io.Reader) (seekableFile, bool) {
	f, ok := in.(seekableFile)
	if !ok {
		return nil, false
	}
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return nil, false
	}
	return f, true
}

// The block sizes. A `read` usually wants one short line, so it starts small
// and doubles while a line runs on; mapfile wants the whole stream and starts
// at the largest.
const (
	blockReadFirst = 128
	blockReadMost  = 64 << 10
)

// blockSource hands out a stream's bytes one at a time while reading it in
// blocks.
type blockSource struct {
	in       io.Reader
	file     seekableFile // nil: nothing to put back, so the stream is read to its end
	buf      []byte
	pos, end int
	size     int
}

// next is a byte source in the shape readSegment takes.
func (s *blockSource) next() (byte, int) {
	if s.pos == s.end && !s.fill() {
		return 0, evEOF
	}
	c := s.buf[s.pos]
	s.pos++
	return c, evByte
}

func (s *blockSource) fill() bool {
	if len(s.buf) < s.size {
		s.buf = make([]byte, s.size)
	}
	// A read that delivered bytes and an error is the bytes now and the end
	// of input at the next fill, which reads again and gets nothing.
	n, _ := s.in.Read(s.buf[:s.size])
	if s.size < blockReadMost {
		s.size *= 2
	}
	s.pos, s.end = 0, max(n, 0)
	return s.end > 0
}

// giveBack moves the file's offset back over the bytes read and not handed
// out, so the next reader of the descriptor starts at the first of them. It
// is idempotent, and costs nothing when there is nothing to give back.
func (s *blockSource) giveBack() {
	unread := s.end - s.pos
	s.pos, s.end = 0, 0
	if unread > 0 && s.file != nil {
		_, _ = s.file.Seek(int64(-unread), io.SeekCurrent)
	}
}

// lineByteSource is the source a line reader takes in: blocks put back on a
// regular file, a byte per call on anything else. done must be called once
// the reader has taken what it wants and before anything else reads the
// stream, and is safe to call more than once.
func lineByteSource(in io.Reader) (next func() (byte, int), done func()) {
	if f, ok := rewindableFile(in); ok {
		s := &blockSource{in: f, file: f, size: blockReadFirst}
		return s.next, s.giveBack
	}
	return directByteSource(in), func() {}
}

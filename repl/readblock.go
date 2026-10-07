// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"bufio"
	"bytes"
	"io"
	"runtime"
)

// lineSource is what the loop without an editor reads its lines from.
type lineSource interface {
	ReadString(delim byte) (string, error)
}

// discarder is a lineSource that can give up what it has read and not yet
// handed over.
type discarder interface {
	discardTheRestOfTheBlock()
}

// plainInput is the reader runPlain takes its lines from: a bufio.Reader,
// which loses nothing, or for the dialect that answers so, a reader whose
// unread block an error throws away. See
// interp.Semantics.PromptErrorDiscardsTheRestOfTheReadBlock.
func (s Shell) plainInput() lineSource {
	if s.ErrorDiscardsTheRestOfTheReadBlock {
		return &readBlocks{r: s.In, size: readBlockSize(runtime.GOOS)}
	}
	return bufio.NewReader(s.In)
}

// discardTheRestOfTheBlock is called where a line was refused or given up,
// and does something only where the input is read in blocks.
func discardTheRestOfTheBlock(in lineSource) {
	if d, ok := in.(discarder); ok {
		d.discardTheRestOfTheBlock()
	}
}

// readBlockSize is the size of one read on the platform named, which is the
// C library's buffer size and not a shell's. Measured 2026-10-07 with dash
// 0.5.12, whose reads are this size: `echo Z` at byte 1023 of a pipe is lost
// after a refused first line and at byte 1024 is run on macOS, and on Debian
// the same edge is at 8192.
func readBlockSize(goos string) int {
	switch goos {
	case "darwin", "ios", "freebsd", "netbsd", "openbsd", "dragonfly":
		return 1024
	default:
		return 8192
	}
}

// readBlocks reads its input one read of size bytes at a time and hands it
// over a line at a time, so that what is left of the current read is a thing
// it can throw away.
//
// One read is one call to Read, which on a pipe is what the writer has put
// there, up to size, and on a terminal is one line: so a terminal, or a
// writer that writes a line at a time and waits, has nothing left over to
// lose.
type readBlocks struct {
	r    io.Reader
	size int
	// buf is what is left of the current block, unhanded.
	buf []byte
	// err is a read's error, kept until what came with it is handed over.
	err error
}

// ReadString returns the input up to and including delim, as
// bufio.Reader.ReadString does: where the input ends first, what there was
// and the error.
//
// The error is not kept once it is returned, so a terminal's ^D, which is an
// end of input that the next read reads past, ends one read and not every
// read after it.
func (b *readBlocks) ReadString(delim byte) (string, error) {
	var out []byte
	for {
		if i := bytes.IndexByte(b.buf, delim); i >= 0 {
			out = append(out, b.buf[:i+1]...)
			b.buf = b.buf[i+1:]
			return string(out), nil
		}
		out = append(out, b.buf...)
		b.buf = nil
		if b.err != nil {
			err := b.err
			b.err = nil
			return string(out), err
		}
		block := make([]byte, b.size)
		n, err := b.r.Read(block)
		b.buf = block[:n]
		b.err = err
	}
}

func (b *readBlocks) discardTheRestOfTheBlock() { b.buf = nil }

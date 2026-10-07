// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"bytes"
	"io"
	"strings"

	"github.com/blairham/sh/internal/lineread"
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

// plainInput is the reader runPlain takes its lines from: exactly a line at
// a time where the dialect takes no more than that, leaving the rest on the
// descriptor for a `read` typed at the prompt; or a block of the dialect's
// size, which an error throws the rest of away where the dialect says so. See
// interp.Semantics.PromptReadSize and
// interp.Semantics.PromptErrorDiscardsTheRestOfTheReadBlock.
func (s Shell) plainInput() lineSource {
	if s.ReadSize > 0 {
		return &readBlocks{r: s.In, size: s.ReadSize}
	}
	return &exactLines{r: s.In, buf: make([]byte, 4096)}
}

// discardTheRestOfTheBlock is called where a line was refused or given up,
// and does something only where the input is read in blocks and the dialect
// throws the rest of one away.
func (s Shell) discardTheRestOfTheBlock(in lineSource) {
	if !s.ErrorDiscardsTheRestOfTheReadBlock {
		return
	}
	if d, ok := in.(discarder); ok {
		d.discardTheRestOfTheBlock()
	}
}

// exactLines is a lineSource that takes nothing past the line it returns,
// which is lineread.Reader with bufio.Reader's way of saying the input ended.
type exactLines struct {
	r   io.Reader
	buf []byte
	lr  lineread.Reader
	// ended is that the last line had no newline, so the next read reports
	// the end without asking the descriptor again.
	ended bool
}

func (l *exactLines) ReadString(byte) (string, error) {
	if l.ended {
		return "", io.EOF
	}
	line := l.lr.Read(l.r, l.buf)
	if !strings.HasSuffix(line, "\n") {
		l.ended = true
		return line, io.EOF
	}
	return line, nil
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

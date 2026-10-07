// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package lineread takes one line at a time off a descriptor that is shared
// with whatever reads it next, leaving the rest where that reader finds it.
package lineread

import (
	"bytes"
	"io"
	"os"
)

// Reader takes one line at a time off the descriptor the program is on,
// leaving it positioned exactly after that line.
//
// *Exactly* after it is the whole requirement, and there are two ways to meet
// it. Where the descriptor can be rewound, read a buffer and push back what
// was not part of the line; where it cannot, never take more than the line in
// the first place. Which of the two is in use is invisible to the script —
// what it finds on descriptor 0 afterwards is the same either way — and that
// was measured rather than assumed: bash, dash, ksh93 and zsh each produce
// byte-identical output for the `read`, `while read`, `cat` and `exec 0<`
// cases whether the program arrives on a pipe or on a regular file.
//
// Two routes share it: a program arriving on standard input, and a prompt
// reading a pipe or a file, where a `read` typed at the prompt or a command it
// starts reads the same descriptor (#6328).
//
// The zero value is ready to use.
type Reader struct {
	// file is the descriptor the answer below is about, and seekable is the
	// answer. Remembered rather than asked per line because asking is a system
	// call of its own and the answer cannot change: whether an open file can
	// be rewound is settled when it is opened. What can change is *which* file
	// is on descriptor 0 — `exec 0< file` puts another one there — so a
	// different file is asked again.
	//
	// The file is held rather than only compared, and that is what the field
	// is for. A closed *os.File can be collected and a later one allocated at
	// the same address; a pipe inheriting a regular file's answer would have
	// this reader take bytes it cannot give back, which is the one failure
	// that matters here. Holding it keeps the address unavailable for as long
	// as the answer is being trusted.
	file     *os.File
	seekable bool
}

// Read returns the next line, its newline included, or what is left where the
// input ends without one; the empty string at the end of input. buf is scratch
// space for the seeking arm, and its size is how much one read asks for there.
func (lr *Reader) Read(in io.Reader, buf []byte) string {
	file, ok := in.(*os.File)
	if !ok {
		// Not a descriptor: a here-document body or whatever an embedder
		// handed the runner. Asking such a reader where it is costs nothing,
		// so it is asked every time rather than remembered.
		if s, ok := in.(io.Seeker); ok {
			if _, err := s.Seek(0, io.SeekCurrent); err == nil {
				return readLineSeeking(in, s, buf)
			}
		}
		return readLineByByte(in)
	}
	if file != lr.file {
		// Asked rather than assumed from the type: a pipe, a socket and a
		// terminal are all *os.File and none of them can be rewound, so the
		// type answers nothing and the descriptor answers everything.
		_, err := file.Seek(0, io.SeekCurrent)
		lr.file, lr.seekable = file, err == nil
	}
	if lr.seekable {
		return readLineSeeking(file, file, buf)
	}
	return readLineByByte(file)
}

// readLineSeeking reads buffers and pushes back everything past the newline it
// found, so that the bytes after the line are still there for whoever reads
// next — the script's own `read`, a command that inherits descriptor 0, or the
// next line of the program.
//
// The push-back is relative, and that is not only cheaper than asking where
// the descriptor is and seeking to a computed place: it is the version that
// cannot be wrong. Between two lines the script has run a command, and a
// command that read descriptor 0 moved it. Anything counted from where a
// previous line ended would put the shell back over bytes it has already
// handed away; an overshoot measured against the read that caused it is right
// wherever that read happened to start.
func readLineSeeking(in io.Reader, s io.Seeker, buf []byte) string {
	// held is what earlier reads produced without reaching a newline. Nil for
	// a line one read covered, which is the ordinary case and the one worth
	// keeping to a single allocation.
	var held []byte
	for {
		n, err := in.Read(buf)
		chunk := buf[:n]
		if i := bytes.IndexByte(chunk, '\n'); i >= 0 {
			if over := int64(n - i - 1); over > 0 {
				if _, err := s.Seek(-over, io.SeekCurrent); err != nil {
					// It could seek a moment ago and cannot now. Nothing can
					// be handed back, so hand it *forward* instead:
					// everything read becomes program text, which is what the
					// block reader does and is the one answer that loses no
					// bytes.
					return string(append(held, chunk...))
				}
			}
			if held == nil {
				return string(chunk[:i+1])
			}
			return string(append(held, chunk[:i+1]...))
		}
		held = append(held, chunk...)
		if err != nil || n == 0 {
			return string(held)
		}
	}
}

// readLineByByte reads up to and including the next newline, a byte at a time.
//
// A byte at a time because the descriptor is shared and cannot be rewound.
// Reading past the line would take input the script is about to ask for —
// which is precisely the difference this whole route turns on, so buffering
// here would reintroduce the bug in a place nobody would think to look for it.
// A pipe is the case, and on a pipe there is no cheaper correct answer: an
// over-read cannot be given back, so it must not happen.
func readLineByByte(in io.Reader) string {
	var b []byte
	var ch [1]byte
	for {
		n, err := in.Read(ch[:])
		if n > 0 {
			b = append(b, ch[0])
			if ch[0] == '\n' {
				return string(b)
			}
		}
		if err != nil {
			return string(b)
		}
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"bytes"
	"io"
	"os"
	"strings"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// program is the script a shell is running and the parser over it, refilled
// from wherever the script is coming from as it is used up.
//
// Two routes end here. A command string, a script file and anything handed to
// Run arrive whole, so there is nothing to refill and this is a parser with a
// loop around it. A script arriving on **standard input** does not: the shell
// holds one descriptor and the script it is running is on the far end of it,
// so how much of it the shell takes at a time is visible to the script itself.
// Taking all of it is the bug this exists to fix — `printf 'read x\necho
// [$x]\ndata\n' | sh` handed the script's own `read` end of input and then ran
// `data` as a command, which is `curl | sh` with the payload eaten.
//
// The pending text is re-parsed rather than resumed when more arrives, and the
// lines already run are skipped by parsing them again and throwing them away.
// That is affordable because pending is only ever one unfinished construct:
// text that parsed cleanly is retired the moment it has, and the line count it
// contributed is carried in base so that a diagnostic names the line of the
// *input* rather than of the piece in hand.
type program struct {
	// src is everything read so far, which is what a diagnostic quotes and
	// what `set -v` echoes. It grows; the lines a `read` took never reach
	// it, and that is why the numbering matches the panel's.
	//
	// A builder rather than a string because a program on standard input
	// arrives a line at a time and `s += line` copies the whole of s each
	// time: a twenty-thousand-line program spent two gigabytes and most of
	// its running time rebuilding text that only a diagnostic ever reads.
	// text() is how it is read, and it costs nothing.
	src strings.Builder

	// more yields the next piece of the input, or false once it has ended.
	// Nil where the whole program was in hand to begin with.
	more func() (string, bool)

	// pending is read but not yet fully parsed, base the number of physical
	// lines before it, and ran the logical lines of it already handed out.
	pending string
	base    int
	ran     int

	dialect syntax.Dialect
	aliases syntax.Aliases

	p *syntax.Parser
	// carried are the remarks of parsers already retired, so that a count of
	// what has been reported stays monotonic across the rebuilds.
	carried []syntax.Remark
}

// wholeProgram is a program whose text is already in hand.
func wholeProgram(src string, d syntax.Dialect) *program {
	pr := &program{pending: src, dialect: d}
	pr.src.WriteString(src)
	return pr
}

// text is everything read so far. Cheap enough to call per line: a builder
// hands back the bytes it already holds rather than copying them.
func (pr *program) text() string { return pr.src.String() }

// setDialect changes the grammar for what has not been read yet, and for every
// parser built after this one: a builtin on one line can decide whether a
// construct on the next is a construct, and the rebuild must not undo it.
func (pr *program) setDialect(d syntax.Dialect) {
	pr.dialect = d
	if pr.p != nil {
		pr.p.SetDialect(d)
	}
}

func (pr *program) parser() *syntax.Parser {
	if pr.p != nil {
		return pr.p
	}
	p := syntax.NewParserAt(pr.pending, pr.dialect, pr.base+1)
	p.Aliases = pr.aliases
	for range pr.ran {
		// Already run. Parsed again only to reach what follows them, and
		// discarded — parsing has no effect of its own.
		p.NextLine()
	}
	pr.p = p
	return p
}

// nextLine parses the next logical line, reading more input if it takes more
// input to finish one.
//
// The second result is false at the end of the program and when parsing
// failed, which err distinguishes.
func (pr *program) nextLine() (*syntax.File, bool) {
	for {
		p := pr.parser()
		line, ok := p.NextLine()
		if p.Incomplete() && pr.fill(false) {
			// The text ran out part-way through a construct that could still
			// be finished, and more of it arrived. Not a failure yet: it is
			// only one when there is nothing left to finish it with.
			continue
		}
		if ok {
			pr.ran++
			return line, true
		}
		if p.Err() != nil {
			return nil, false
		}
		// Everything in hand has been parsed. Whatever comes next comes from
		// the descriptor as it stands *now*, which is how `exec 0< file`
		// mid-script replaces the rest of the script in three of the four.
		if !pr.fill(true) {
			return nil, false
		}
	}
}

// err reports why parsing stopped, or nil.
func (pr *program) err() error {
	if pr.p == nil {
		return nil
	}
	return pr.p.Err()
}

// remarks is what every parser over this program has had to say about input it
// accepted anyway.
func (pr *program) remarks() []syntax.Remark {
	if pr.p == nil {
		return pr.carried
	}
	return append(pr.carried[:len(pr.carried):len(pr.carried)], pr.p.Remarks()...)
}

// fill reads the next piece of the input, reporting whether there was one.
//
// retire says the pending text was parsed through to its end, so it can be
// dropped and only its line count kept. Without it the piece is appended to
// what is already there and the whole is parsed again, which is what an
// unfinished construct needs.
func (pr *program) fill(retire bool) bool {
	if pr.more == nil {
		return false
	}
	if retire {
		if pr.p != nil {
			pr.carried = append(pr.carried, pr.p.Remarks()...)
		}
		// The lines of the text, plus the lines the alias bodies expanded in
		// it added to the numbering — which are in no text at all, so
		// counting newlines cannot find them and the shift would be lost at
		// every refill.
		pr.base += strings.Count(pr.pending, "\n")
		if pr.p != nil {
			pr.base += pr.p.LineShift()
		}
		pr.pending, pr.ran = "", 0
	}
	text, ok := pr.more()
	if !ok {
		pr.more = nil
		return false
	}
	pr.take(text)
	// A line ending in a backslash is joined to the one after it, and only
	// the reader knows whether there is one — see syntax.EndsWithContinuation.
	// Asked before anything is parsed, because a command that looks finished
	// would otherwise run before the line completing it was read.
	for syntax.EndsWithContinuation(pr.pending) {
		next, ok := pr.more()
		if !ok {
			pr.more = nil
			break
		}
		pr.take(next)
	}
	pr.p = nil
	return true
}

func (pr *program) take(text string) {
	pr.pending += text
	pr.src.WriteString(text)
}

// stdinProgram reads a script off the descriptor the shell was invoked with,
// as the shell runs it.
//
// From r.In() on every call rather than from a reader captured once: the
// script's own standard input *is* the script, so `exec 0< file` half way
// through replaces the rest of the program, which is measured in bash, ksh93
// and zsh alike.
//
// blocks is the dialect's answer to how much to take at a time. See
// interp.Semantics.StdinProgramReadInBlocks — a line leaves the rest of the
// input where the script can reach it, and a block does not.
//
// The buffer is made once and reused, and so is the reader's memory of what
// kind of descriptor it is looking at. Both readers hand back a fresh string
// and keep nothing, so there is nothing for two calls to tread on, and a
// program of twenty thousand lines is twenty thousand allocations otherwise.
func stdinProgram(r *interp.Runner, blocks bool) func() (string, bool) {
	buf := make([]byte, blockSize)
	var lines lineReader
	return func() (string, bool) {
		var text string
		if blocks {
			text = readBlock(r.In(), buf)
		} else {
			text = lines.read(r.In(), buf)
		}
		return text, text != ""
	}
}

// lineReader takes one line at a time off the descriptor the program is on,
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
// This is the fetch and not the unit. Semantics.StdinProgramReadInBlocks
// decides how much of the input the shell is entitled to take; this decides
// how many system calls it costs to take it.
type lineReader struct {
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

func (lr *lineReader) read(in io.Reader, buf []byte) string {
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

// blockSize is how much a shell reading in blocks asks for at once, and how
// much the line reader takes before rewinding. The exact figure is not a
// behavior anyone can depend on — where a block boundary falls is a fact about
// the producer's timing, and a rewound over-read is not observable at all — so
// this is simply a buffer.
const blockSize = 8192

// readBlock takes as much as the descriptor will give in one go, and then
// enough more to finish the line it stopped in the middle of.
//
// Finishing the line matters: a producer is free to split its bytes anywhere,
// and a half-written command is not a command. The shell that reads this way
// waits for the rest of it too.
//
// It does not rewind, on a seekable descriptor or any other. Keeping what it
// took is what this reader *is* — measured, dash reads a program the same way
// from a file as from a pipe, and its `read` finds end of input either way.
func readBlock(in io.Reader, buf []byte) string {
	var b []byte
	for {
		n, err := in.Read(buf)
		b = append(b, buf[:n]...)
		if err != nil || n == 0 {
			return string(b)
		}
		if b[len(b)-1] == '\n' {
			return string(b)
		}
	}
}

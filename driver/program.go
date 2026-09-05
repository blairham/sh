// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"io"
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
	src string

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
	return &program{src: src, pending: src, dialect: d}
}

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
		pr.base += strings.Count(pr.pending, "\n")
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
	pr.src += text
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
func stdinProgram(r *interp.Runner, blocks bool) func() (string, bool) {
	return func() (string, bool) {
		var text string
		if blocks {
			text = readBlock(r.In())
		} else {
			text = readLine(r.In())
		}
		return text, text != ""
	}
}

// readLine reads up to and including the next newline, a byte at a time.
//
// A byte at a time because the descriptor is shared. Reading past the line
// would take input the script is about to ask for — which is precisely the
// difference this whole route turns on, so buffering here would reintroduce
// the bug in a place nobody would think to look for it.
func readLine(in io.Reader) string {
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

// blockSize is how much a shell reading in blocks asks for at once. The exact
// figure is not a behavior anyone can depend on — where the boundary falls is
// a fact about the producer's timing — so this is simply a buffer.
const blockSize = 8192

// readBlock takes as much as the descriptor will give in one go, and then
// enough more to finish the line it stopped in the middle of.
//
// Finishing the line matters: a producer is free to split its bytes anywhere,
// and a half-written command is not a command. The shell that reads this way
// waits for the rest of it too.
func readBlock(in io.Reader) string {
	buf := make([]byte, blockSize)
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

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"bytes"
	"io"
	"strings"

	"github.com/blairham/sh/internal/lineread"
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
	// The three alias hooks the parser takes, kept together because they are
	// one feature with three tables — see interp.Runner.ExpandingAlias.
	aliases       syntax.Aliases
	globalAliases syntax.Aliases
	suffixAliases syntax.Aliases

	p *syntax.Parser
	// carried are the remarks of parsers already retired, so that a count of
	// what has been reported stays monotonic across the rebuilds.
	carried []syntax.Remark

	// gate is the history expander, once a script has asked for one. Nil for
	// every program that never writes `set -o history`, which is the point:
	// see driver/historygate.go, where the design and what it replaced are
	// written down.
	gate *histGate
}

// handOver puts the text the program has not run yet behind a history gate,
// and feeds the parser through it from here on.
//
// at is the byte offset, in everything read so far, of the first line the
// program has not run — which the front end has in hand as the `set -v`
// position, because the echo has already walked past every line that ran.
//
// The whole-text parse is retired exactly as fill does it, and for the same
// reason: what has run is spent, and only its line count has to survive so
// that a diagnostic names the line of the *file*.
//
// The source text is rewound to the same point and re-accumulates from the
// gate, so what `set -v` writes back is the **expanded** line. That is
// measured rather than chosen: bash under `set -v; set -H` echoes `echo echo
// one two three` where the file holds `echo !!`.
func (pr *program) handOver(g *histGate, at int) {
	text := pr.text()
	at = min(max(at, 0), len(text))
	k := min(max(at-(len(text)-len(pr.pending)), 0), len(pr.pending))
	pr.base += strings.Count(pr.pending[:k], "\n")
	if pr.p != nil {
		pr.carried = append(pr.carried, pr.p.Remarks()...)
		pr.base += pr.p.LineShift()
	}
	g.at = pr.base
	g.rest, g.more = pr.pending[k:], pr.more
	pr.gate, pr.more = g, g.next
	pr.pending, pr.ran, pr.p = "", 0, nil
	head := text[:at]
	pr.src.Reset()
	pr.src.WriteString(head)
}

// recordHistory puts the command the parser has just handed back into the
// list, where a program has a gate collecting one.
//
// Called by the front end between the parse and the run, because that is
// where bash has it: a command is in the list before it executes, so
// `history` lists itself and a `history -s` appends *after* its own line.
func (pr *program) recordHistory() {
	if pr.gate != nil {
		pr.gate.record()
	}
}

// openQuote is what the parser is still inside, which is what the next
// physical line begins inside. Empty where there is no parser yet or where it
// finished what it was given.
func (pr *program) openQuote() (open string, heredocExpands bool) {
	if pr.p == nil {
		return "", false
	}
	return pr.p.OpenQuote(), pr.p.OpenHeredocExpands()
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
	p.GlobalAliases = pr.globalAliases
	p.SuffixAliases = pr.suffixAliases
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

// nothingAfter is whether the line just handed out is the last of a program
// that arrived whole: what follows it is blank lines and comments, or
// nothing. False wherever more input could still arrive, since a pipe's next
// read is not text in hand.
func (pr *program) nothingAfter(line *syntax.File) bool {
	if pr.more != nil || pr.gate != nil || line == nil {
		return false
	}
	at := int(line.Last.Offset)
	if at < 0 || at > len(pr.pending) {
		return false
	}
	for _, rest := range strings.Split(pr.pending[at:], "\n") {
		rest = strings.TrimLeft(rest, " \t")
		if rest != "" && !strings.HasPrefix(rest, "#") {
			return false
		}
	}
	return true
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
	if pr.gate != nil {
		// What the next physical line begins inside, and whether the line
		// before it finished a command. The gate is the program's `more` at
		// this point, so both reach it before it reads anything.
		pr.gate.open, pr.gate.heredocExpands = pr.openQuote()
		pr.gate.flush = retire
		pr.gate.dialect = pr.dialect
	}
	// Read first and retire second. Retiring hands the parser's remarks to
	// carried, and a retire that then finds no more input leaves the same
	// remarks in *both* places — `remarks` reads carried and the parser it
	// did not replace, so the last one is said twice. Nothing exhibited that
	// until a remark arrived that a whole one-line program can raise: a
	// program that ends part-way through a construct never gets here, which
	// is the shape the here-document remark has.
	text, ok := pr.more()
	if !ok {
		pr.more = nil
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
	pr.take(text)
	// A line ending in a backslash is joined to the one after it, and only
	// the reader knows whether there is one — see syntax.EndsWithContinuation.
	// Asked before anything is parsed, because a command that looks finished
	// would otherwise run before the line completing it was read.
	for syntax.EndsWithContinuation(pr.pending) {
		if pr.gate != nil {
			// The line about to be read is joined to the one before it here
			// and not by the parser, so nothing else will tell the gate what
			// that line begins inside — and the answer decides how the
			// history entry joins the pair. Measured: bash 5.3.20 records
			// `echo \` / `A` as the one line `echo A` and `echo "a\` / `b"`
			// as the two the file holds, backslash and all.
			//
			// Asked of a parser over the text in hand, for the reason the
			// gate states for the whole question: `$'`, a backquote, a `'`
			// inside a double-quoted string and a here-document body are
			// four answers the lexer already has. Only ever reached by a
			// line ending in a backslash.
			pr.gate.open, pr.gate.heredocExpands = openIn(pr.pending, pr.dialect)
		}
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

// openIn is what a parser is still inside having read this much text.
func openIn(text string, d syntax.Dialect) (open string, heredocExpands bool) {
	p := syntax.NewParser(text, d)
	p.Parse()
	return p.OpenQuote(), p.OpenHeredocExpands()
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
// size is the dialect's answer to how much to take at a time. See
// interp.Semantics.StdinProgramReadSize — a line leaves the rest of the
// input where the script can reach it, and a block does not.
//
// The buffer is made once and reused, and so is the reader's memory of what
// kind of descriptor it is looking at. Both readers hand back a fresh string
// and keep nothing, so there is nothing for two calls to tread on, and a
// program of twenty thousand lines is twenty thousand allocations otherwise.
func stdinProgram(r *interp.Runner, size interp.ReadSize) func() (string, bool) {
	blocks := size.Bytes() > 0
	buf := make([]byte, lineBufferSize)
	var block blockReader
	if blocks {
		block.buf = make([]byte, size.Bytes())
	}
	var lines lineReader
	return func() (string, bool) {
		var text string
		if blocks {
			text = block.read(r.In())
		} else {
			text = lines.read(r.In(), buf)
		}
		return text, text != ""
	}
}

// lineReader takes one line at a time off the descriptor the program is on,
// leaving it positioned exactly after that line. See lineread.Reader, which
// the prompt reading a pipe shares it with.
//
// This is the fetch and not the unit. Semantics.StdinProgramReadSize
// decides how much of the input the shell is entitled to take; this decides
// how many system calls it costs to take it.
type lineReader struct{ lr lineread.Reader }

func (lr *lineReader) read(in io.Reader, buf []byte) string { return lr.lr.Read(in, buf) }

// lineBufferSize is how much the line reader takes before rewinding, on a
// descriptor that can be rewound. A rewound over-read is not observable at
// all, so this is simply a buffer; a block's size is observable, and is the
// dialect's (#6329).
const lineBufferSize = 8192

// blockReader takes one read's worth of the descriptor at a time and hands
// over the whole lines in it, keeping the line it stopped in the middle of for
// the next call.
//
// The lines it hands over run before anything more is read, and that is
// measured: with `read x`, blank lines, `DATA` starting one byte before the
// end of dash's first block and `echo "[$x]"`, dash 0.5.12 runs `read x` with
// only the `D` taken, so the `read` gets `ATA`, and the next block makes the
// line `Decho "[$x]"`, a command not found. BusyBox ash 1.37.0 does the same
// at its own block's edge (#6329). Finishing the line before running what
// came before it had the `read` find the end of input instead.
//
// A half-written command is still not a command: the partial line waits for
// the rest of it, reading block after block where one line is longer than a
// block.
//
// It does not rewind, on a seekable descriptor or any other. Keeping what it
// took is what this reader *is* — measured, dash reads a program the same way
// from a file as from a pipe, and its `read` finds end of input either way.
type blockReader struct {
	buf []byte
	// carry is the start of a line a read stopped in the middle of.
	carry []byte
}

func (b *blockReader) read(in io.Reader) string {
	for {
		n, err := in.Read(b.buf)
		data := append(b.carry, b.buf[:n]...)
		b.carry = nil
		if err != nil || n == 0 {
			return string(data)
		}
		i := bytes.LastIndexByte(data, '\n')
		if i < 0 {
			b.carry = data
			continue
		}
		b.carry = append([]byte(nil), data[i+1:]...)
		return string(data[:i+1])
	}
}

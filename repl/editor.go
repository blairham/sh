// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

// Reading a line the way a shell does.
//
// The terminal is in raw mode, so the bytes arrive as they are typed and
// nothing is echoed: this draws the line itself. That is not decoration. The
// moment the cursor can move, the terminal's own echo is wrong — it would
// print an arrow key rather than move — so a shell that offers editing at all
// has to own what is on the screen.
//
// Runes rather than bytes throughout, because a cursor sits between characters
// and not between the halves of one.

// ErrInterrupted is a line the user abandoned with ^C. The line is discarded
// and the prompt comes back; it is not an error the shell should report.
var ErrInterrupted = errors.New("interrupted")

// editor holds the line being typed and the history behind it.
type editor struct {
	in  io.Reader
	out io.Writer

	// line is what has been typed, and pos is where the cursor sits in it —
	// an index *between* runes, so 0 is before the first and len(line) is
	// after the last.
	line []rune
	pos  int

	// history is every line accepted so far, oldest first. browsing is where
	// Up and Down have walked to: len(history) means "not browsing, on the
	// line being typed", and stash holds that line while browsing so Down
	// can bring it back.
	history  []string
	browsing int
	stash    []rune
}

// readLine reads one line, drawing it as it is typed.
//
// It returns io.EOF for ^D on an empty line — which is how a shell is told to
// exit — and ErrInterrupted for ^C, which abandons the line without exiting.
func (e *editor) readLine(prompt string) (string, error) {
	e.line, e.pos = e.line[:0], 0
	e.browsing = len(e.history)
	e.write(prompt)

	var buf [1]byte
	for {
		n, err := e.in.Read(buf[:])
		if err != nil {
			return "", err
		}
		if n == 0 {
			continue
		}
		switch c := buf[0]; c {
		case ctrlC:
			// The line is abandoned, not run. The newline is ours to print:
			// the terminal echoes nothing in raw mode, so without it the
			// next prompt would land on top of what was typed.
			e.write("^C\r\n")
			return "", ErrInterrupted
		case ctrlD:
			if len(e.line) == 0 {
				e.write("\r\n")
				return "", io.EOF
			}
			// With something typed, ^D deletes forwards instead — which is
			// what it means everywhere but on an empty line.
			e.deleteForward()
		case '\r', '\n':
			e.write("\r\n")
			return string(e.line), nil
		case ctrlA:
			e.moveTo(0, prompt)
		case ctrlE:
			e.moveTo(len(e.line), prompt)
		case ctrlB:
			e.moveTo(e.pos-1, prompt)
		case ctrlF:
			e.moveTo(e.pos+1, prompt)
		case ctrlK:
			e.line = e.line[:e.pos]
			e.redraw(prompt)
		case ctrlU:
			e.line = append([]rune(nil), e.line[e.pos:]...)
			e.pos = 0
			e.redraw(prompt)
		case ctrlW:
			e.deleteWord()
			e.redraw(prompt)
		case ctrlL:
			// Clear the screen and put the line back at the top of it.
			e.write("\x1b[H\x1b[2J")
			e.redraw(prompt)
		case ctrlP:
			e.browse(-1, prompt)
		case ctrlN:
			e.browse(+1, prompt)
		case backspace, del:
			e.deleteBackward()
			e.redraw(prompt)
		case esc:
			e.escape(prompt)
		default:
			if c < 0x20 {
				// Any other control character is ignored rather than
				// inserted: a shell that put a raw byte in the line would
				// hand the parser something no one typed.
				continue
			}
			r, err := e.readRune(c)
			if err != nil {
				return "", err
			}
			e.insert(r)
			e.redraw(prompt)
		}
	}
}

// readRune completes a UTF-8 sequence whose first byte has arrived.
//
// The terminal delivers bytes, and a character outside ASCII arrives as
// several. Inserting them one at a time would put half a rune in the line and
// draw a cursor between the halves.
func (e *editor) readRune(first byte) (rune, error) {
	buf := []byte{first}
	for !utf8.FullRune(buf) && len(buf) < utf8.UTFMax {
		var next [1]byte
		n, err := e.in.Read(next[:])
		if err != nil {
			return 0, err
		}
		if n == 0 {
			continue
		}
		buf = append(buf, next[0])
	}
	r, _ := utf8.DecodeRune(buf)
	return r, nil
}

// escape reads what follows an ESC and acts on the arrows it recognizes.
//
// An unrecognized sequence is dropped rather than inserted. A terminal sends
// far more than this understands, and putting the bytes in the line would mean
// a stray function key ended up in the command.
func (e *editor) escape(prompt string) {
	var b [1]byte
	if n, err := e.in.Read(b[:]); err != nil || n == 0 || b[0] != '[' {
		return
	}
	if n, err := e.in.Read(b[:]); err != nil || n == 0 {
		return
	}
	switch b[0] {
	case 'A':
		e.browse(-1, prompt)
	case 'B':
		e.browse(+1, prompt)
	case 'C':
		e.moveTo(e.pos+1, prompt)
	case 'D':
		e.moveTo(e.pos-1, prompt)
	case 'H':
		e.moveTo(0, prompt)
	case 'F':
		e.moveTo(len(e.line), prompt)
	case '3':
		// Delete arrives as ESC [ 3 ~, so the tilde has to be eaten too.
		if n, err := e.in.Read(b[:]); err == nil && n > 0 && b[0] == '~' {
			e.deleteForward()
			e.redraw(prompt)
		}
	}
}

// browse walks the history. -1 is older, +1 is newer.
//
// The line being typed is put aside on the first step back and returned when
// the walk reaches the end again, so a half-written command is not lost to a
// glance at what came before.
func (e *editor) browse(dir int, prompt string) {
	if len(e.history) == 0 {
		return
	}
	to := e.browsing + dir
	if to < 0 || to > len(e.history) {
		return
	}
	if e.browsing == len(e.history) {
		e.stash = append([]rune(nil), e.line...)
	}
	e.browsing = to
	if to == len(e.history) {
		e.line = append([]rune(nil), e.stash...)
	} else {
		e.line = []rune(e.history[to])
	}
	e.pos = len(e.line)
	e.redraw(prompt)
}

// remember adds an accepted line to the history.
//
// Blank lines and an immediate repeat are not kept: they are what a history
// is most often cluttered with, and neither is worth walking back through.
func (e *editor) remember(line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	if n := len(e.history); n > 0 && e.history[n-1] == line {
		return
	}
	e.history = append(e.history, line)
}

func (e *editor) insert(r rune) {
	e.line = append(e.line, 0)
	copy(e.line[e.pos+1:], e.line[e.pos:])
	e.line[e.pos] = r
	e.pos++
}

func (e *editor) deleteBackward() {
	if e.pos == 0 {
		return
	}
	e.line = append(e.line[:e.pos-1], e.line[e.pos:]...)
	e.pos--
}

func (e *editor) deleteForward() {
	if e.pos >= len(e.line) {
		return
	}
	e.line = append(e.line[:e.pos], e.line[e.pos+1:]...)
}

// deleteWord removes the word before the cursor, and the run of spaces
// before it — which is what makes ^W usable on `a  b` rather than needing two.
func (e *editor) deleteWord() {
	i := e.pos
	for i > 0 && e.line[i-1] == ' ' {
		i--
	}
	for i > 0 && e.line[i-1] != ' ' {
		i--
	}
	e.line = append(e.line[:i], e.line[e.pos:]...)
	e.pos = i
}

func (e *editor) moveTo(pos int, prompt string) {
	if pos < 0 || pos > len(e.line) {
		return
	}
	e.pos = pos
	e.redraw(prompt)
}

// redraw puts the whole line back on the screen.
//
// The whole line each time rather than the difference. A shell that tracked
// what changed would be faster and would be wrong the first time a character
// is wider than one cell or the line wraps; and at typing speed there is
// nothing to gain.
func (e *editor) redraw(prompt string) {
	var b strings.Builder
	b.WriteString("\r\x1b[K")
	b.WriteString(prompt)
	b.WriteString(string(e.line))
	if e.pos < len(e.line) {
		// Back up to where the cursor belongs, in characters rather than
		// bytes.
		b.WriteString("\x1b[")
		b.WriteString(itoa(len(e.line) - e.pos))
		b.WriteString("D")
	}
	e.write(b.String())
}

func (e *editor) write(s string) { _, _ = io.WriteString(e.out, s) }

// itoa without importing strconv for one call on the keystroke path.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// The control bytes this editor acts on, named so the switch above reads as
// what it does rather than as a table of numbers.
const (
	ctrlA     = 0x01
	ctrlB     = 0x02
	ctrlC     = 0x03
	ctrlD     = 0x04
	ctrlE     = 0x05
	ctrlF     = 0x06
	ctrlK     = 0x0b
	ctrlL     = 0x0c
	ctrlN     = 0x0e
	ctrlP     = 0x10
	ctrlU     = 0x15
	ctrlW     = 0x17
	esc       = 0x1b
	backspace = 0x08
	del       = 0x7f
)

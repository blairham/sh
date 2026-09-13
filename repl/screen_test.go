// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"unicode/utf8"
)

// A terminal, enough of one to say what the editor put on the screen.
//
// The editor's output is instructions and not a picture, and for most of this
// package's life the tests read the instructions: a row asserted that the
// bytes contained `\r\x1b[4C`, which is one way of arriving at column four and
// not the only one. That was fine while there was one way — and it stopped
// being fine the moment the draw learned to send a difference, because a
// relative move to the same cell failed a row that nothing was wrong with.
//
// **The instruction is not the claim; the screen is.** So the tests that care
// where the cursor ends up, or what the line looks like, run the bytes through
// this and ask the screen. It is stricter than a substring and not weaker: a
// substring can be satisfied by bytes that also do something else, and this
// cannot.
//
// It understands what the editor emits and nothing more — printable text with
// the deferred wrap a real terminal has, `\r`, `\n`, the four cursor moves,
// the two erases, and SGR, which it records rather than renders.
type screen struct {
	cells [][]rune
	// paint is the style in force in each cell, the same shape as cells.
	//
	// Per cell and not a list of the sequences that went past, because
	// *where* a color landed is the whole question for a draw that re-sends
	// only part of a line: a difference that moved the text and left the
	// color behind writes every sequence the whole-line draw writes, in the
	// same order, onto the wrong characters. A row that only asked whether
	// the line had been colored would pass for exactly that.
	paint [][]string
	row   int
	col   int
	cols  int
	// pen is the style in force now.
	pen    string
	styled []string
}

func newScreen(cols int) *screen {
	return &screen{cells: [][]rune{{}}, paint: [][]string{{}}, cols: cols}
}

func (s *screen) at(row int) []rune {
	for len(s.cells) <= row {
		s.cells = append(s.cells, []rune{})
	}
	for len(s.paint) <= row {
		s.paint = append(s.paint, []string{})
	}
	return s.cells[row]
}

func (s *screen) put(r rune) {
	// A terminal holds the wrap until there is something to put on the next
	// row, which is the whole reason the editor writes a space and a carriage
	// return at the edge. Modeling it any other way would make that code
	// look wrong.
	if s.cols > 0 && s.col >= s.cols {
		s.row, s.col = s.row+1, 0
	}
	line := s.at(s.row)
	for len(line) <= s.col {
		line = append(line, ' ')
	}
	line[s.col] = r
	s.cells[s.row] = line
	paint := s.paint[s.row]
	for len(paint) <= s.col {
		paint = append(paint, "")
	}
	paint[s.col] = s.pen
	s.paint[s.row] = paint
	s.col += runeWidth(r)
}

// feed writes bytes to the screen the way a terminal would read them.
func (s *screen) feed(out string) *screen {
	for i := 0; i < len(out); {
		c := out[i]
		switch {
		case c == '\r':
			s.col = 0
			i++
		case c == '\n':
			s.row++
			s.at(s.row)
			i++
		case c == 0x1b && i+1 < len(out) && out[i+1] == '[':
			j := i + 2
			for j < len(out) && (out[j] < '@' || out[j] > '~') {
				j++
			}
			if j >= len(out) {
				return s
			}
			s.csi(out[i+2:j], out[j])
			i = j + 1
		default:
			r, w := utf8.DecodeRuneInString(out[i:])
			s.put(r)
			i += w
		}
	}
	return s
}

func (s *screen) csi(args string, verb byte) {
	n := 1
	if args != "" {
		n = 0
		for _, c := range args {
			if c < '0' || c > '9' {
				n = 1
				break
			}
			n = n*10 + int(c-'0')
		}
	}
	switch verb {
	case 'A':
		if s.row -= n; s.row < 0 {
			s.row = 0
		}
	case 'B':
		s.row += n
		s.at(s.row)
	case 'C':
		s.col += n
	case 'D':
		if s.col -= n; s.col < 0 {
			s.col = 0
		}
	case 'K':
		line := s.at(s.row)
		if s.col < len(line) {
			s.cells[s.row] = line[:s.col]
			s.paint[s.row] = s.paint[s.row][:min(s.col, len(s.paint[s.row]))]
		}
	case 'J':
		line := s.at(s.row)
		if s.col < len(line) {
			s.cells[s.row] = line[:s.col]
			s.paint[s.row] = s.paint[s.row][:min(s.col, len(s.paint[s.row]))]
		}
		s.cells = s.cells[:s.row+1]
		s.paint = s.paint[:s.row+1]
	case 'm':
		seq := "\x1b[" + args + "m"
		if seq == highlightReset {
			s.pen = ""
		} else {
			s.pen = seq
		}
		s.styled = append(s.styled, seq)
	}
}

// text is what the screen shows, rows joined by newlines and trailing blanks
// removed — what a person would read off it.
func (s *screen) text() string {
	rows := make([]string, 0, len(s.cells))
	for _, line := range s.cells {
		rows = append(rows, strings.TrimRight(string(line), " "))
	}
	return strings.TrimRight(strings.Join(rows, "\n"), "\n")
}

// cursor is where the cursor was left.
func (s *screen) cursor() (row, col int) { return s.row, s.col }

// painted reports whether any SGR sequence reached the screen.
func (s *screen) painted() bool { return len(s.styled) > 0 }

// colors is the style in force under each character the screen shows, rows
// joined the way text does, so two screens can be compared on where the color
// landed and not merely on whether there was some.
//
// A letter per distinct style, assigned in the order the styles first appear.
// The first spelling of this hashed the style's *length*, which made red and
// green — five bytes each — the same letter, and a row that could not tell two
// colors apart cannot catch a color left in the wrong place. That is the
// failure this whole model exists to catch, so it is worth saying twice: an
// instrument that collapses the thing it is measuring reads exactly like one
// that found nothing wrong.
func (s *screen) colors() string {
	seen := map[string]byte{}
	var b strings.Builder
	for row, line := range s.cells {
		if row > 0 {
			b.WriteByte('\n')
		}
		trimmed := []rune(strings.TrimRight(string(line), " "))
		for col := range trimmed {
			style := ""
			if row < len(s.paint) && col < len(s.paint[row]) {
				style = s.paint[row][col]
			}
			if style == "" {
				b.WriteByte('.')
				continue
			}
			letter, ok := seen[style]
			if !ok {
				letter = byte('A' + len(seen))
				seen[style] = letter
			}
			b.WriteByte(letter)
		}
	}
	return b.String()
}

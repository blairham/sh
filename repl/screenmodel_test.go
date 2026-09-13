// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// A terminal, enough of one to say what the editor's drawing put on a screen.
//
// It exists because the redraw stopped being a shape that can be asserted on.
// While every keystroke rewrote the whole line, "came back up to the prompt
// row" and "moved the cursor four columns in" were substrings of the output
// and a test could look for them. An incremental redraw writes whichever of
// several equivalent sequences is shortest for the change in hand — a
// backwards move where a carriage return and a walk out is longer, no move at
// all where the cursor is already in the right cell — so a test that names one
// of them is asserting a coincidence.
//
// What is *not* a coincidence is the screen. Every one of those tests was
// really about where the characters and the cursor end up, so that is what
// this makes assertable: feed it what the editor wrote and ask what a terminal
// would be showing.
//
// Deliberately small. It knows the sequences this package emits and nothing
// else, and an unknown one is ignored rather than guessed at — a model that
// invented an answer would fail a test for a reason that is not in the editor.
//
// **It records the color of every cell as well as the character in it**, and
// that is not decoration. A model that kept only the characters said a screen
// was right while the highlighting on it was wrong, and for as long as it did,
// TestReopeningAQuoteRecolorsWhatWasAlreadyDrawn was green over a redraw that
// left the recolored word plain — the defect in #2627, in the test written
// for exactly that case. Stripping the styling before looking is the same
// blind spot as stripping ANSI before asserting on output: what the test is
// about is the first thing thrown away.
type screen struct {
	cols int
	rows [][]rune
	// style is the attributes each cell was drawn under, cell for cell with
	// rows. Empty is the terminal's default.
	style [][]string
	// sgr is what is in force now. A terminal keeps one of these for the
	// whole screen and a cursor move does not touch it, which is the whole of
	// why a redraw cannot resume inside a colored run.
	sgr string
	row int
	col int
	// pending is the deferred wrap: a terminal that has filled a row stays on
	// it until there is another character to put somewhere. Modeling it is
	// not optional — it is the whole of why the editor writes a space and a
	// carriage return after a line that ends at the edge.
	pending bool
}

func newScreen(cols int) *screen { return &screen{cols: cols} }

// feed plays the bytes a terminal was sent.
func (s *screen) feed(out string) {
	for i := 0; i < len(out); {
		switch out[i] {
		case '\r':
			s.col, s.pending = 0, false
			i++
		case '\n':
			s.row, s.pending = s.row+1, false
			i++
		case '\a':
			i++
		case '\b':
			s.col, s.pending = max(0, s.col-1), false
			i++
		case esc:
			i += s.control(out[i:])
		default:
			r, n := utf8.DecodeRuneInString(out[i:])
			s.put(r)
			i += n
		}
	}
}

// control acts on one escape sequence and answers how many bytes it was.
func (s *screen) control(out string) int {
	if len(out) < 2 || out[1] != '[' {
		// Not a CSI. Skip the escape and whatever single byte follows it,
		// which is what displayWidth does with one.
		if len(out) < 2 {
			return 1
		}
		return 2
	}
	j := 2
	for j < len(out) && (out[j] < '@' || out[j] > '~') {
		j++
	}
	if j >= len(out) {
		return len(out)
	}
	n := 1
	if params := out[2:j]; params != "" {
		n = 0
		for _, c := range params {
			if c < '0' || c > '9' {
				n = 1
				break
			}
			n = n*10 + int(c-'0')
		}
	}
	switch out[j] {
	case 'A':
		s.row, s.pending = max(0, s.row-n), false
	case 'B':
		s.row, s.pending = s.row+n, false
	case 'C':
		s.col, s.pending = min(s.cols-1, s.col+n), false
	case 'D':
		s.col, s.pending = max(0, s.col-n), false
	case 'H':
		s.row, s.col, s.pending = 0, 0, false
	case 'J':
		s.eraseBelow()
	case 'K':
		s.eraseRow()
	case 'm':
		// Select Graphic Rendition. No parameters and a lone zero are both
		// "everything back to default"; anything else adds to what is in
		// force, because that is how a terminal composes them and how a
		// highlighter emitting a color and a weight expects them to compose.
		if params := out[2:j]; params == "" || params == "0" {
			s.sgr = ""
		} else {
			s.sgr += out[:j+1]
		}
	}
	return j + 1
}

func (s *screen) put(r rune) {
	w := runeWidth(r)
	if s.pending {
		s.row, s.col, s.pending = s.row+1, 0, false
	}
	if w > 0 && s.col+w > s.cols {
		// A terminal will not split a wide character across the edge; it
		// wraps early and leaves the last cell blank.
		s.row, s.col = s.row+1, 0
	}
	s.grow(s.row)
	if w == 0 {
		return
	}
	s.rows[s.row][s.col] = r
	s.style[s.row][s.col] = s.sgr
	for k := 1; k < w; k++ {
		// The cells a wide character covers hold nothing of their own.
		s.rows[s.row][s.col+k] = 0
		s.style[s.row][s.col+k] = s.sgr
	}
	s.col += w
	if s.col >= s.cols {
		s.col, s.pending = s.cols, true
	}
}

func (s *screen) grow(row int) {
	for len(s.rows) <= row {
		blank := make([]rune, s.cols)
		for i := range blank {
			blank[i] = ' '
		}
		s.rows = append(s.rows, blank)
		s.style = append(s.style, make([]string, s.cols))
	}
}

func (s *screen) eraseRow() {
	s.grow(s.row)
	for i := min(s.col, s.cols); i < s.cols; i++ {
		s.rows[s.row][i] = ' '
		// An erased cell is blank and carries no attributes. A terminal with
		// background-color erase would paint it with whatever is in force,
		// which is why the redraw writes a reset before erasing; a model that
		// kept the old attributes here would be asserting the opposite.
		s.style[s.row][i] = ""
	}
}

func (s *screen) eraseBelow() {
	s.eraseRow()
	s.rows = s.rows[:min(s.row+1, len(s.rows))]
	s.style = s.style[:min(s.row+1, len(s.style))]
}

// text is what is on the screen, one row per line, with trailing blanks gone.
func (s *screen) text() string {
	var out []string
	for _, row := range s.rows {
		var b strings.Builder
		for _, r := range row {
			if r == 0 {
				continue
			}
			b.WriteRune(r)
		}
		out = append(out, strings.TrimRight(b.String(), " "))
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}

// styledText is what is on the screen with the colors in it, one row per
// line, written back out as the shortest escapes that would produce them.
//
// Canonical rather than a replay: two ways of saying one color compare equal
// here, which is the point — a redraw is free to emit whichever sequence is
// shorter, and what is being asserted is the screen it produced.
func (s *screen) styledText() string {
	var out []string
	for r := range s.rows {
		last := -1
		for c, ch := range s.rows[r] {
			if ch != ' ' && ch != 0 {
				last = c
			}
		}
		var b strings.Builder
		cur := ""
		for c := 0; c <= last; c++ {
			ch := s.rows[r][c]
			if ch == 0 {
				continue
			}
			if st := s.style[r][c]; st != cur {
				if st == "" {
					b.WriteString(highlightReset)
				} else {
					b.WriteString(st)
				}
				cur = st
			}
			b.WriteRune(ch)
		}
		if cur != "" {
			b.WriteString(highlightReset)
		}
		out = append(out, b.String())
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}

// at is where the cursor is, with the deferred wrap resolved the way the
// editor's own arithmetic resolves it — see pastEdge.
func (s *screen) at() (row, col int) {
	if s.pending {
		return s.row + 1, 0
	}
	return s.row, s.col
}

// shownBy is the screen a terminal would be showing after being sent out.
func shownBy(cols int, out string) *screen {
	s := newScreen(cols)
	s.feed(out)
	return s
}

// A sanity check on the model itself, because a model that draws nothing would
// make every test below pass.
func TestTheScreenModelDrawsWhatATerminalWould(t *testing.T) {
	for _, tc := range []struct {
		name, in string
		cols     int
		want     string
		row, col int
	}{
		{"plain", "$ hi", 20, "$ hi", 0, 4},
		{"colors take no cells", "$ \x1b[32mhi\x1b[0m", 20, "$ hi", 0, 4},
		{"wrap", "$ 123456789", 10, "$ 12345678\n9", 1, 1},
		{"deferred wrap", "$ 12345678", 10, "$ 12345678", 1, 0},
		{"back up and over", "$ hi\x1b[1A\r\x1b[2C", 20, "$ hi", 0, 2},
		{"erase below", "$ 123456789\x1b[1A\r\x1b[2C\x1b[J", 10, "$", 0, 2},
		{"wide", "$ 日本語", 10, "$ 日本語", 0, 8},
		{"wide wraps early", "$ 日本語日本", 10, "$ 日本語日\n本", 1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := shownBy(tc.cols, tc.in)
			if got := s.text(); got != tc.want {
				t.Errorf("screen is\n%q\nwant\n%q", got, tc.want)
			}
			if row, col := s.at(); row != tc.row || col != tc.col {
				t.Errorf("cursor at row %d column %d, want row %d column %d", row, col, tc.row, tc.col)
			}
		})
	}
}

// drawnScreen plays a sequence of edits through one editor and answers the
// screen the last of them left, without the line ever being accepted.
//
// Accepting it would put a newline and whatever follows on the screen, and
// every question here is about the screen a line is being *typed* on.
func drawnScreen(t *testing.T, cols int, prompt string, steps []edit) *screen {
	t.Helper()
	var out strings.Builder
	e := &editor{out: &out, width: func() int { return cols }}
	p := drawPrompt(prompt)
	for _, step := range steps {
		e.line, e.pos = []rune(step.line), step.pos
		e.redraw(p)
	}
	return shownBy(cols, out.String())
}

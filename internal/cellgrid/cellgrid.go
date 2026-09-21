// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package cellgrid turns what a terminal was sent into what it would be
// showing: a grid of cells, each with the grapheme in it and the appearance
// it is drawn under.
//
// # Why a grid and not a string
//
// docs/spec/prompt-theme.md asks for the fidelity comparison to be over
// **rendered cells** and states the reason:
//
//	Two different SGR spellings paint the same screen — `38;5;31` and `31`
//	are the same color, and an attribute can be set in either order — so a
//	byte diff fails on prompts that are identical to look at.
//
// So the parameters are *resolved* rather than kept as text. `\x1b[31m`,
// `\x1b[38;5;1m` and `\x1b[0;1;38;5;1m` after a bold all reach the same
// foreground here, and two attributes set in either order reach the same
// cell. That is what makes a difference this package reports a difference a
// person could see.
//
// # And never by stripping
//
// The other half of the spec's rule is that nothing here discards
// appearance: *"a harness that compares plain text cannot tell a working
// theme from a colorless one, which is the blind spot that has hidden broken
// rendering in this tree before."* A colorless render and a colored one
// differ in every colored cell, and that is the point of the package.
//
// # Deliberately small, and an unknown sequence is skipped rather than
// guessed at
//
// It knows the sequences a prompt is drawn with: the cursor moves, the
// erases, the wrap, and SGR. Anything else is consumed and ignored, which is
// the same call repl's own screen model makes for the same reason — a model
// that invented an answer would report a difference that is not on anybody's
// screen.
//
// It is not a terminal emulator and is not offered as one: no scroll region,
// no alternate screen, no character sets, no mouse. Each is absent because a
// prompt does not use it.
package cellgrid

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ColorKind says how a color was specified, which is what makes two
// spellings of one color compare equal.
type ColorKind uint8

const (
	// ColorDefault is the terminal's own, which is what an unset color and
	// SGR 39 or 49 both mean.
	ColorDefault ColorKind = iota
	// ColorIndex is a palette entry, 0-255. The 30-37 and 90-97 ranges
	// resolve here too, because they name the same slots: it is the same
	// color on every terminal that supports either spelling, and a
	// comparison that told them apart would fail on prompts that look
	// identical.
	ColorIndex
	// ColorRGB is a 24-bit triple.
	ColorRGB
)

// Color is a resolved color.
type Color struct {
	Kind    ColorKind
	Index   uint8
	R, G, B uint8
}

// String names a color the way a difference report reads best.
func (c Color) String() string {
	switch c.Kind {
	case ColorIndex:
		return strconv.Itoa(int(c.Index))
	case ColorRGB:
		return "#" + hex2(c.R) + hex2(c.G) + hex2(c.B)
	}
	return "default"
}

func hex2(v uint8) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[v>>4], digits[v&0xf]})
}

// Style is the appearance a cell is drawn under.
//
// The attributes a prompt uses and no others. A terminal has more — blink,
// conceal, strikethrough — and a prompt that used one would be reported as a
// difference in nothing, which is a gap worth having written down rather
// than a set worth padding.
type Style struct {
	Fg, Bg    Color
	Bold      bool
	Faint     bool
	Italic    bool
	Underline bool
	Reverse   bool
}

// Cell is one position on the screen.
type Cell struct {
	// Text is the grapheme in the cell: a rune and whatever combining marks
	// followed it, so an emoji with a modifier is one cell's worth of text
	// rather than two cells.
	Text string
	Style
}

// Blank reports whether a cell has nothing drawn in it.
func (c Cell) Blank() bool { return c.Text == "" || c.Text == " " }

// Grid is what a terminal of a given width would be showing.
type Grid struct {
	cols  int
	rows  [][]Cell
	style Style
	row   int
	col   int
	// pending is the deferred wrap: a terminal that has filled a row stays
	// on it until there is another character to put somewhere. Modeling it
	// is not optional — it is why an editor writes a space and a carriage
	// return after a line that ends at the edge.
	pending bool
	// partial is a UTF-8 sequence split across two writes, which happens
	// whenever the bytes arrive from a pipe.
	partial []byte
}

// New returns an empty grid of the given width.
func New(cols int) *Grid {
	if cols <= 0 {
		cols = 80
	}
	return &Grid{cols: cols}
}

// Cols is the width the grid was built at.
func (g *Grid) Cols() int { return g.cols }

// Rows is how many rows have been touched.
func (g *Grid) Rows() int { return len(g.rows) }

// Cell is what is at a position, or a blank cell past the end.
func (g *Grid) Cell(row, col int) Cell {
	if row < 0 || row >= len(g.rows) || col < 0 || col >= len(g.rows[row]) {
		return Cell{}
	}
	return g.rows[row][col]
}

// Text is one row's graphemes, with the trailing blanks removed.
func (g *Grid) Text(row int) string {
	if row < 0 || row >= len(g.rows) {
		return ""
	}
	var b strings.Builder
	for _, cell := range g.rows[row] {
		if cell.Text == "" {
			b.WriteString(" ")
			continue
		}
		b.WriteString(cell.Text)
	}
	return strings.TrimRight(b.String(), " ")
}

// String is every row's text, one per line — for a failure message, never
// for a comparison. Comparing this is exactly the blind spot the package
// exists to close.
func (g *Grid) String() string {
	lines := make([]string, len(g.rows))
	for i := range g.rows {
		lines[i] = g.Text(i)
	}
	return strings.Join(lines, "\n")
}

// Write plays bytes a terminal was sent.
func (g *Grid) Write(p []byte) (int, error) {
	n := len(p)
	if len(g.partial) > 0 {
		p = append(g.partial, p...)
		g.partial = nil
	}
	for i := 0; i < len(p); {
		switch {
		case p[i] == '\r':
			g.col, g.pending = 0, false
			i++
		case p[i] == '\n':
			g.moveTo(g.row+1, g.col)
			g.pending = false
			i++
		case p[i] == '\b':
			if g.col > 0 {
				g.col--
			}
			g.pending = false
			i++
		case p[i] == '\t':
			g.put(" ")
			for g.col%8 != 0 {
				g.put(" ")
			}
			i++
		case p[i] == 0x1b:
			used, incomplete := g.escape(p[i:])
			if incomplete {
				g.partial = append(g.partial[:0], p[i:]...)
				return n, nil
			}
			i += used
		case p[i] < 0x20 || p[i] == 0x7f:
			// A control byte a prompt has no use for — the non-printing
			// markers an editor wraps escapes in among them. Skipped rather
			// than drawn: a marker that reached the screen would be a cell
			// the terminal never showed.
			i++
		default:
			r, size := utf8.DecodeRune(p[i:])
			if r == utf8.RuneError && size == 1 && !utf8.FullRune(p[i:]) {
				g.partial = append(g.partial[:0], p[i:]...)
				return n, nil
			}
			i += size
			g.appendRune(r)
		}
	}
	return n, nil
}

// appendRune draws a rune, joining a combining mark onto the cell before it.
//
// A grapheme cluster and not a rune per cell, because the spec's comparison
// is over "the grapheme, foreground, background and attributes" — an emoji
// with a modifier and a letter with an accent each occupy one cell and a
// comparison that split them would report a difference in the encoding
// rather than in the screen.
func (g *Grid) appendRune(r rune) {
	if combining(r) && g.col > 0 && g.row < len(g.rows) {
		prev := g.col - 1
		if g.pending {
			prev = g.cols - 1
		}
		if prev >= 0 && prev < len(g.rows[g.row]) && g.rows[g.row][prev].Text != "" {
			g.rows[g.row][prev].Text += string(r)
			return
		}
	}
	g.put(string(r))
}

// combining reports whether a rune joins the grapheme before it.
func combining(r rune) bool {
	return unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) ||
		r == 0x200d || (r >= 0xfe00 && r <= 0xfe0f)
}

// put writes one grapheme in the current style and advances.
func (g *Grid) put(text string) {
	if g.pending {
		g.moveTo(g.row+1, 0)
		g.pending = false
	}
	g.ensure(g.row, g.col)
	g.rows[g.row][g.col] = Cell{Text: text, Style: g.style}
	if g.col+1 >= g.cols {
		g.pending = true
		return
	}
	g.col++
}

// moveTo puts the cursor somewhere, growing the grid to reach it.
func (g *Grid) moveTo(row, col int) {
	if row < 0 {
		row = 0
	}
	if col < 0 {
		col = 0
	}
	if col >= g.cols {
		col = g.cols - 1
	}
	g.row, g.col = row, col
	g.ensure(row, col)
}

// ensure grows the grid so a position exists.
func (g *Grid) ensure(row, col int) {
	for len(g.rows) <= row {
		g.rows = append(g.rows, make([]Cell, g.cols))
	}
	_ = col
}

// escape reads one escape sequence, answering how many bytes it took and
// whether it was cut short by the end of the buffer.
func (g *Grid) escape(p []byte) (used int, incomplete bool) {
	if len(p) < 2 {
		return 0, true
	}
	switch p[1] {
	case '[':
		return g.csi(p)
	case ']':
		// An operating-system command — a window title, a hyperlink. It ends
		// at a BEL or an ST, and it paints nothing, so it is consumed whole.
		for i := 2; i < len(p); i++ {
			if p[i] == 0x07 {
				return i + 1, false
			}
			if p[i] == 0x1b && i+1 < len(p) && p[i+1] == '\\' {
				return i + 2, false
			}
		}
		return 0, true
	case '(', ')', '*', '+', '#':
		if len(p) < 3 {
			return 0, true
		}
		return 3, false
	}
	return 2, false
}

// csi reads a control sequence.
func (g *Grid) csi(p []byte) (used int, incomplete bool) {
	i := 2
	private := false
	if i < len(p) && (p[i] == '?' || p[i] == '>' || p[i] == '<' || p[i] == '=') {
		private = true
		i++
	}
	start := i
	for i < len(p) && (p[i] >= '0' && p[i] <= '9' || p[i] == ';' || p[i] == ':') {
		i++
	}
	if i >= len(p) {
		return 0, true
	}
	params := string(p[start:i])
	final := p[i]
	i++
	if private {
		// A mode set or reset — bracketed paste, the cursor's visibility.
		// None of them paints a cell.
		return i, false
	}
	g.apply(final, params)
	return i, false
}

// apply performs one control sequence.
func (g *Grid) apply(final byte, params string) {
	n := func(def int) int {
		first, _, _ := strings.Cut(params, ";")
		v, err := strconv.Atoi(first)
		if err != nil || v == 0 && def != 0 {
			return def
		}
		return v
	}
	switch final {
	case 'A':
		g.moveTo(g.row-n(1), g.col)
		g.pending = false
	case 'B':
		g.moveTo(g.row+n(1), g.col)
		g.pending = false
	case 'C':
		g.moveTo(g.row, g.col+n(1))
		g.pending = false
	case 'D':
		g.moveTo(g.row, g.col-n(1))
		g.pending = false
	case 'G':
		g.moveTo(g.row, n(1)-1)
		g.pending = false
	case 'H', 'f':
		row, col, _ := strings.Cut(params, ";")
		g.moveTo(atoiOr(row, 1)-1, atoiOr(col, 1)-1)
		g.pending = false
	case 'J':
		g.eraseDisplay(n(0))
	case 'K':
		g.eraseLine(n(0))
	case 'm':
		g.sgr(params)
	}
}

// eraseLine clears part of the row the cursor is on.
func (g *Grid) eraseLine(mode int) {
	g.ensure(g.row, 0)
	row := g.rows[g.row]
	switch mode {
	case 1:
		for i := 0; i <= g.col && i < len(row); i++ {
			row[i] = Cell{}
		}
	case 2:
		for i := range row {
			row[i] = Cell{}
		}
	default:
		for i := g.col; i < len(row); i++ {
			row[i] = Cell{}
		}
	}
}

// eraseDisplay clears part of the screen.
func (g *Grid) eraseDisplay(mode int) {
	g.ensure(g.row, 0)
	switch mode {
	case 1:
		for r := 0; r < g.row; r++ {
			clear(g.rows[r])
		}
		g.eraseLine(1)
	case 2, 3:
		for r := range g.rows {
			clear(g.rows[r])
		}
	default:
		g.eraseLine(0)
		for r := g.row + 1; r < len(g.rows); r++ {
			clear(g.rows[r])
		}
	}
}

// sgr resolves an appearance change.
//
// The whole of the package's claim is here: a parameter is *resolved* into a
// Style rather than remembered as text, so two spellings of one color and
// two orderings of two attributes reach the same cell.
func (g *Grid) sgr(params string) {
	if params == "" {
		g.style = Style{}
		return
	}
	fields := strings.Split(params, ";")
	for i := 0; i < len(fields); i++ {
		code := atoiOr(fields[i], 0)
		switch {
		case code == 0:
			g.style = Style{}
		case code == 1:
			g.style.Bold = true
		case code == 2:
			g.style.Faint = true
		case code == 3:
			g.style.Italic = true
		case code == 4:
			g.style.Underline = true
		case code == 7:
			g.style.Reverse = true
		case code == 21 || code == 22:
			g.style.Bold, g.style.Faint = false, false
		case code == 23:
			g.style.Italic = false
		case code == 24:
			g.style.Underline = false
		case code == 27:
			g.style.Reverse = false
		case code >= 30 && code <= 37:
			g.style.Fg = Color{Kind: ColorIndex, Index: uint8(code - 30)}
		case code >= 90 && code <= 97:
			g.style.Fg = Color{Kind: ColorIndex, Index: uint8(code - 90 + 8)}
		case code >= 40 && code <= 47:
			g.style.Bg = Color{Kind: ColorIndex, Index: uint8(code - 40)}
		case code >= 100 && code <= 107:
			g.style.Bg = Color{Kind: ColorIndex, Index: uint8(code - 100 + 8)}
		case code == 39:
			g.style.Fg = Color{}
		case code == 49:
			g.style.Bg = Color{}
		case code == 38 || code == 48:
			color, used := extended(fields[i+1:])
			i += used
			if code == 38 {
				g.style.Fg = color
			} else {
				g.style.Bg = color
			}
		}
	}
}

// extended reads the parameters after a 38 or 48.
func extended(rest []string) (Color, int) {
	if len(rest) == 0 {
		return Color{}, 0
	}
	switch atoiOr(rest[0], 0) {
	case 5:
		if len(rest) < 2 {
			return Color{}, len(rest)
		}
		return Color{Kind: ColorIndex, Index: uint8(atoiOr(rest[1], 0))}, 2
	case 2:
		if len(rest) < 4 {
			return Color{}, len(rest)
		}
		return Color{
			Kind: ColorRGB,
			R:    uint8(atoiOr(rest[1], 0)),
			G:    uint8(atoiOr(rest[2], 0)),
			B:    uint8(atoiOr(rest[3], 0)),
		}, 4
	}
	return Color{}, 1
}

func atoiOr(s string, def int) int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return v
}

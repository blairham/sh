// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package cellgrid

import (
	"fmt"
	"strings"
)

// Comparing two screens, cell by cell.
//
// This is what "looks exactly the same" means, stated so a machine can check
// it: per cell, the grapheme, the foreground, the background and the
// attributes. Everything that makes two spellings of one appearance equal
// was done when the grid was built; what is left here is a walk.

// Difference is one cell that differs, or one row present on one side only.
type Difference struct {
	Row, Col int
	Want     Cell
	Got      Cell
}

// String renders a difference the way a report reads it.
func (d Difference) String() string {
	return fmt.Sprintf("row %d col %d: want %s, got %s",
		d.Row+1, d.Col+1, describe(d.Want), describe(d.Got))
}

// describe says what a cell holds, appearance and all.
//
// Appearance always, even where the graphemes differ, because the failure
// this instrument exists for is a prompt that draws the right text in the
// wrong color — and a report that printed only the text when the text
// differed would hide the case where both do.
func describe(c Cell) string {
	text := c.Text
	if text == "" {
		text = " "
	}
	var attrs []string
	if c.Bold {
		attrs = append(attrs, "bold")
	}
	if c.Faint {
		attrs = append(attrs, "faint")
	}
	if c.Italic {
		attrs = append(attrs, "italic")
	}
	if c.Underline {
		attrs = append(attrs, "underline")
	}
	if c.Reverse {
		attrs = append(attrs, "reverse")
	}
	out := fmt.Sprintf("%q fg=%s bg=%s", text, c.Fg, c.Bg)
	if len(attrs) > 0 {
		out += " " + strings.Join(attrs, "+")
	}
	return out
}

// Compare walks two grids and answers every cell that differs.
//
// **A blank cell and an untouched cell are the same thing**, and that is not
// a loosening. One side may reach a column by writing a space and the other
// by never going there, and a terminal shows nothing either way — so a
// comparison that told them apart would report a difference nobody can see,
// which is the mirror of the byte diff this replaces. A blank cell carrying
// a *background* is not blank: that is paint, and it is what a framed prompt
// is made of.
//
// Rows are compared to the length of the longer side, so a row one grid has
// and the other does not is reported cell by cell rather than as one line
// nobody can locate.
func Compare(want, got *Grid) []Difference {
	rows := max(want.Rows(), got.Rows())
	cols := max(want.Cols(), got.Cols())
	var out []Difference
	for row := range rows {
		for col := range cols {
			a, b := want.Cell(row, col), got.Cell(row, col)
			if sameCell(a, b) {
				continue
			}
			out = append(out, Difference{Row: row, Col: col, Want: a, Got: b})
		}
	}
	return out
}

// sameCell is the comparison one cell at a time.
func sameCell(a, b Cell) bool {
	if a.Style != b.Style {
		// Except where neither cell paints anything: an untouched cell and a
		// space are the same screen, and an untouched cell's style is the
		// zero value while the space may have been written under one that
		// shows nothing.
		if !invisible(a) || !invisible(b) {
			return false
		}
	}
	return text(a) == text(b)
}

// invisible reports whether a cell shows nothing at all — no grapheme and no
// paint. A background makes a blank cell visible, which is what a framed
// prompt's whitespace is.
func invisible(c Cell) bool {
	return c.Blank() && c.Bg.Kind == ColorDefault && !c.Reverse
}

func text(c Cell) string {
	if c.Text == "" {
		return " "
	}
	return c.Text
}

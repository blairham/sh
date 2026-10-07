// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "strings"

// Drawing a change the way bash's editor does: in place, with the terminal's
// own insert and delete sequences.
//
// The default repaint (repaint.go) writes everything from the first changed
// character to the end of the line again and erases what is left over. bash
// 5.3.20 compares the old line with the new from both ends, and changes only
// the middle. Measured 2026-10-07 through a pseudo-terminal, `--norc -i`,
// `PS1='P> '`, 80 columns (#6332):
//
//	                               xterm             vt52 / vt100        wy50          tvi912
//	X inserted before `def`        \e[1@X            Xdef + back 3       \eq \er\bX    \eQX
//	Backspace mid-line             \b\e[1P           \bdef\eK + back 3   \b\eW         \b\eW
//	Backspace at the end           \b\e[K            \b\eK               \b\eT         \b\eT
//	Up, `echo thre` → `echo one`   \b×4\e[1Pon\e[C   \b×4one\eK          \b×4\eWon^L   \b×4\eWon^L
//	^N, back                       \b×3\e[1@thr\e[C  \b×3thre            smir, ' ', …  \b×3\eQthr^L
//	^W, `th` before `re`           \b\b\e[2P         \b\bre\eK\b\b       \b\b\eW\eW    \b\b\eW\eW
//	^A on `echo abc`               \r\e[C\e[C\e[C    \r\eC\eC\eC
//	^E from col 3 to 11            \e[C ×8           \eC ×8
//
// and under adm3a, which has no `el`, the old tail is covered with spaces.
//
// Read off those and the rows that pin the edges:
//
//   - The common prefix and the common suffix of the old line and the new are
//     left alone; the cursor moves to the end of the prefix.
//   - **Inserting** in front of a suffix — the new middle longer than the old —
//     opens the room first and then writes the new middle: `ich` with the
//     count, else `ich1` that many times, else insert mode (`smir`, as many
//     spaces, `rmir`, back over them). Only where the old line has at least
//     two characters from the end of the prefix: one character typed in front
//     of a single `y` is `ay\b`, in front of `yy` it is `\e[1@a`.
//   - **Deleting** in front of a suffix: `dch` with the count, else `dch1`
//     that many times, and then the new middle — only while twice the suffix
//     is at least the number deleted. `^W` taking four characters before `yy`
//     is `\e[4P`, taking five is the tail written again.
//   - Otherwise the rest of the line is written again from the end of the
//     prefix, and a line that got shorter is erased with `el` (no reset in
//     front of it) or, with no `el`, covered with spaces.
//   - **Left** is a return to the row's start and `cuf1` to the place when the
//     place is nearer the start than the cursor, and `cub1` repeated
//     otherwise: ^A from column 6 to 3 is three backspaces, from 11 it is
//     `\r` and three `\e[C`.
//   - **Right** is `cuf1` repeated, whatever the terminal's counted move.

// canInsert and canDelete say what a terminal offers for drawing a change in
// place.
func (m *terminalMotion) canInsert() bool {
	return m.insert != "" || m.insert1 != "" || (m.insertOn != "" && m.insertOff != "")
}

func (m *terminalMotion) canDelete() bool {
	return m.delete != "" || m.delete1 != ""
}

// openRoom writes what makes n columns of room at the cursor, leaving it where
// it was.
func (m *terminalMotion) openRoom(b *strings.Builder, n int) {
	switch {
	case m.insert != "":
		b.WriteString(m.counted(m.insert, n))
	case m.insert1 != "":
		b.WriteString(m.repeated(m.insert1, n))
	default:
		b.WriteString(m.out(m.insertOn))
		b.WriteString(strings.Repeat(" ", n))
		b.WriteString(m.out(m.insertOff))
		b.WriteString(strings.Repeat(m.stepLeft(), n))
	}
}

// closeUp deletes n columns at the cursor.
func (m *terminalMotion) closeUp(b *strings.Builder, n int) {
	if m.delete != "" {
		b.WriteString(m.counted(m.delete, n))
		return
	}
	b.WriteString(m.repeated(m.delete1, n))
}

// columnAsReadline moves along one row the way the comment at the top of this
// file says.
func (m *terminalMotion) columnAsReadline(b *strings.Builder, fromCol, toCol int) {
	step := func(n int) {
		if s := m.repeated(m.right1, n); s != "" || n == 0 {
			b.WriteString(s)
			return
		}
		b.WriteString(m.rightBy(n))
	}
	switch {
	case toCol == fromCol:
	case toCol > fromCol:
		step(toCol - fromCol)
	case toCol < fromCol-toCol:
		b.WriteString("\r")
		step(toCol)
	default:
		b.WriteString(strings.Repeat(m.stepLeft(), fromCol-toCol))
	}
}

// repaintInPlace is repaint for a shell that draws a change in place, for a
// line on one row with nothing styled in it. It reports false where it does
// not apply, and the caller draws the way it otherwise would.
func (e *editor) repaintInPlace(m *terminalMotion, prompt drawnPrompt, cols int) bool {
	d := e.drawn
	shown := e.displayed()
	if d.endRow != 0 || e.listingBelow || d.right || strings.ContainsRune(d.styled, esc) {
		return false
	}
	styled := e.styled()
	if styled != string(shown) || strings.ContainsRune(styled, esc) {
		return false
	}
	old, cur := []rune(d.styled), shown
	for _, r := range old {
		if isControl(r) || r == '\n' || runeWidth(r) != 1 {
			return false
		}
	}
	for _, r := range cur {
		if isControl(r) || r == '\n' || runeWidth(r) != 1 {
			return false
		}
	}
	if prompt.cells+len(cur) >= cols || prompt.cells+len(old) >= cols {
		return false
	}
	p := 0
	for p < len(old) && p < len(cur) && old[p] == cur[p] {
		p++
	}
	s := 0
	for s < len(old)-p && s < len(cur)-p && old[len(old)-1-s] == cur[len(cur)-1-s] {
		s++
	}
	lo, ln := len(old)-p-s, len(cur)-p-s

	var b strings.Builder
	col := d.col
	if lo != 0 || ln != 0 || len(old) != len(cur) {
		m.columnAsReadline(&b, col, prompt.cells+p)
		col = prompt.cells + p
		middle := string(cur[p : p+ln])
		switch {
		case ln > lo && s > 0 && lo+s >= 2 && m.canInsert():
			m.openRoom(&b, ln-lo)
			b.WriteString(middle)
			col += ln
		case lo > ln && s > 0 && 2*s >= lo-ln && m.canDelete():
			m.closeUp(&b, lo-ln)
			b.WriteString(middle)
			col += ln
		default:
			b.WriteString(string(cur[p:]))
			col = prompt.cells + len(cur)
			if gone := len(old) - len(cur); gone > 0 {
				if el := m.eraseToRowEnd(); el != "" {
					b.WriteString(el)
				} else {
					b.WriteString(strings.Repeat(" ", gone))
					col += gone
				}
			}
		}
	}
	to := prompt.cells + e.pos
	m.columnAsReadline(&b, col, to)

	e.row = 0
	if b.Len() > 0 {
		e.write(b.String())
	}
	end := prompt.cells + len(cur)
	e.drawn = drawnLine{
		valid:  true,
		styled: styled,
		prompt: prompt.text,
		cells:  prompt.cells,
		cols:   cols,
		row:    0, col: to,
		endRow: 0, endCol: end,
	}
	return true
}

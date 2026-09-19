// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"unicode/utf8"
)

// Redrawing only what changed.
//
// The editor used to put the whole line back on the screen for every
// keystroke: return to the start of the prompt's row, erase to the end of the
// screen, write the prompt again, write the line again, and walk the cursor
// back. That is O(line) bytes per character where a real line editor is
// O(change), and it was cheap only while a line was plain text. Once a
// highlighter is in front of `self-insert` the line carries an escape sequence
// per token and a reset after each, and once the prompt is one of the themes
// people actually run it is several hundred bytes of escapes on its own —
// which a keystroke has no reason to touch at all.
//
// Measured 2026-09-13 through a pseudo-terminal, against zsh 5.9.2 driven the
// same way, at the same width, under the same rc file: a themed prompt and a
// highlighter wrapping `self-insert`. Seven keystrokes of each shape, counting
// the bytes the shell wrote:
//
//	                      zsh 5.9.2   whole-line   this file
//	typing at the end           142         1764          63
//	inserting mid-line        1,090        2,184         588
//	deleting                    140          938          72
//	a wrapped line              162        3,258          63
//	cursor motion                 7        1,064           7
//
// The whole-line column depends on the prompt and the other two do not: with a
// plain `%# ` prompt in place of the theme it is 450 rather than 1764, because
// most of what it wrote was the prompt. zsh does not rewrite the prompt for a
// keystroke and neither does this.
//
// Those figures predate [styleInForce], which is a correction and not free: a
// keystroke landing *inside* a colored run now carries the run's escape as
// well as the character, because the terminal has to be put back into a state
// the shared prefix does not leave it in. Measured 2026-09-13 on the built
// shell under `-highlight`, typing the eight characters of `"one two` into an
// unclosed quotation: 80 bytes, against 45 for the draws that were losing the
// color and 130 for redrawing the run from its opening sequence. A keystroke
// outside a run — which is every keystroke on a line with no highlighting on
// it, and the rows above — is untouched.
//
// **What makes it safe is that the editor knows exactly what it last drew.**
// The state below is written only by a redraw, and [editor.write] clears it —
// so anything else that puts bytes on the terminal, a completion listing, a
// search line, a fresh prompt, a cleared screen, drops the editor back to the
// whole-line draw on the next keystroke. There is no path that has to remember
// to invalidate, because the invalidation is on the write itself.
type drawnLine struct {
	// valid says the fields below describe what is on the screen right now.
	valid bool

	// styled is the line exactly as it was written, escape sequences and all.
	// The comparison is against these bytes rather than against the line and
	// its runs, because two draws that emit the same bytes put the same
	// characters in the same cells whatever produced them.
	//
	// **They do not leave the terminal in the same *style*, and that is not
	// what the shared prefix says.** The color in force is whatever the last
	// byte of the *previous whole draw* set, not whatever the shared prefix
	// would have set had it been written on its own — the cursor came back
	// over the line afterwards and a cursor move carries no attributes. A
	// prefix ending inside a highlighted run therefore names a screen
	// position where the terminal is already back at its default, and the
	// bytes after it were written expecting the run to be in force. See
	// styleInForce, which is where the run is said again before the resume
	// (#2627).
	styled string

	// prompt and cells are the prompt this was drawn under. A prompt whose
	// text changed is a screen this cannot reason about: the line starts
	// somewhere else, and the old prompt is still on the row.
	prompt string
	cells  int

	// cols is the width it was drawn at, because every row and column below
	// was counted against it.
	cols int

	// right says a right prompt is on the row. It is not a position, because
	// a right prompt does not have one that can change: it is pinned to the
	// edge and only its *presence* moves. So this is the whole of what an
	// incremental repaint has to check — see repaint, which declines rather
	// than trying to put one on or take one off.
	right bool

	// row and col are where the draw left the cursor, and endRow and endCol
	// are where the drawn content ends. Rows are counted from the row the
	// prompt starts on, the way [place] counts them.
	//
	// Both are normalized: a column equal to the width is the edge with the
	// wrap still pending, and it is recorded as column 0 of the row below,
	// because that is the cell the next character goes in either way.
	row, col       int
	endRow, endCol int
}

// repaint redraws only the part of the line that changed, and reports whether
// it could.
//
// It cannot when the screen is not the one it last drew — a different width, a
// different prompt, or anything at all written since. The caller then puts the
// whole line back, which is always correct and is what this used to be.
func (e *editor) repaint(prompt drawnPrompt, cols int) bool {
	d := e.drawn
	if !d.valid || d.cols != cols || d.cells != prompt.cells || d.prompt != prompt.text {
		return false
	}
	if rightFits(prompt.cells, cells(e.line), prompt.rightCells, cols) != d.right {
		// The line has just grown into the right prompt, or shrunk back off
		// it. An incremental repaint writes what changed and erases nothing,
		// so it cannot take one off; the whole-line draw clears to the end of
		// the screen before it writes, so it can do both. Declining here is
		// the whole of the interaction between the two.
		return false
	}

	styled := e.styled()
	at, resume := sharedPrefix(d.styled, styled)
	if resume > len(e.line) {
		// The highlighter emitted more characters than the line has, which
		// styled does not do. Rather than reason about a screen this cannot
		// account for, fall back.
		return false
	}

	resRow, resCol := placeAt(prompt.cells, e.line, resume, cols)
	curRow, curCol, endRow, endCol := place(prompt.cells, e.line, e.pos, cols)
	resRow, resCol = pastEdge(resRow, resCol, cols)
	curRow, curCol = pastEdge(curRow, curCol, cols)

	// row and col follow the cursor through the write, so that every move
	// below is from where it actually is. Nothing has been written yet, so
	// that is where the last draw left it.
	var b strings.Builder
	row, col := d.row, d.col
	if tail := styled[at:]; tail != "" {
		moveCursor(&b, row, col, resRow, resCol)
		// The color the tail was written expecting, said again. See
		// styleInForce: the shared prefix names a cell, not a state, and the
		// state the terminal is actually in is the one the last whole draw
		// left.
		b.WriteString(styleInForce(styled, at))
		// Spelled for the terminal rather than for the line: a newline in the
		// line, which only a paste puts there, is a line feed on its own and
		// leaves the cursor in the column it was in. See onScreen, and place,
		// which counts the rows this makes.
		b.WriteString(onScreen(tail))
		row, col = endRow, endCol
		if endCol == cols {
			// The content ends exactly at the right-hand edge, where a
			// terminal stays on the row it filled until there is something
			// else to put somewhere. A space makes the wrap happen and the
			// carriage return undoes the space.
			b.WriteString(" \r")
			row, col = endRow+1, 0
		}
	}
	// An empty tail is a redraw that changed no bytes — a cursor motion, or a
	// widget that touched nothing. The cursor is not moved to the end for one:
	// it is already where it was left, and walking it out to the end of the
	// line and back is two movements to accomplish one.
	endRow, endCol = pastEdge(endRow, endCol, cols)
	if d.endRow > endRow || (d.endRow == endRow && d.endCol > endCol) {
		// The line got shorter, so there is a tail of the old one still on the
		// screen. Erase from the new end to the end of the screen rather than
		// to the end of the row: what is left over may be several rows of it.
		//
		// The reset first because the erase paints with the current
		// attributes on a terminal with background-color erase, and the
		// cursor may be sitting inside a highlighted run whose style the
		// shared prefix left in force.
		moveCursor(&b, row, col, endRow, endCol)
		row, col = endRow, endCol
		b.WriteString(highlightReset)
		b.WriteString("\x1b[J")
	}
	moveCursor(&b, row, col, curRow, curCol)

	e.row = curRow
	if b.Len() > 0 {
		// Nothing to say is the line and the runs both unchanged with the
		// cursor where it already was — a redraw asked for by a widget that
		// changed neither. It costs no bytes at all.
		e.write(b.String())
	}
	e.drawn = drawnLine{
		valid:  true,
		styled: styled,
		prompt: prompt.text,
		cells:  prompt.cells,
		cols:   cols,
		right:  d.right,
		row:    curRow, col: curCol,
		endRow: endRow, endCol: endCol,
	}
	return true
}

// promptDrawn records the screen a freshly written prompt leaves, so that the
// first keystroke of a line writes the character and not the prompt again.
//
// It is worth its own call because the prompt is the expensive half. A theme
// of the kind people run is several hundred bytes of escape sequences, and
// without this the first key of every line pays for all of them — measured at
// 126 bytes against 10 for the keys after it.
//
// Nothing is recorded for a prompt wider than the terminal. The rest of this
// package counts the line's rows from the row the prompt's *last* row begins
// on and takes that to be the row the cursor is on, which a prompt that wraps
// on its own makes untrue; the whole-line redraw is what has always handled
// that case and it goes on handling it.
func (e *editor) promptDrawn(prompt drawnPrompt) {
	cols := e.cols()
	if cols <= 0 || prompt.cells >= cols {
		return
	}
	e.drawn = drawnLine{
		valid:  true,
		prompt: prompt.text,
		cells:  prompt.cells,
		cols:   cols,
		// A fresh prompt has an empty line under it, so whether a right
		// prompt was drawn is decided by the prompt's own width alone.
		right: rightFits(prompt.cells, 0, prompt.rightCells, cols),
		row:   0, col: prompt.cells,
		endRow: 0, endCol: prompt.cells,
	}
}

// pastEdge turns a column at the right-hand edge into the start of the row
// below.
//
// A column equal to the width is a terminal that has filled a row and not yet
// wrapped, which is not a cell the cursor can be *moved* to — and the cell the
// next character lands in is the first of the row below either way. Every
// position this file compares or moves to goes through here, so that two ways
// of naming one cell never compare unequal.
func pastEdge(row, col, cols int) (int, int) {
	if col == cols {
		return row + 1, 0
	}
	return row, col
}

// sharedPrefix is how much of two drawn lines is byte-identical: how many
// bytes, and how many of the line's own characters those bytes drew.
//
// Bytes rather than runs, because byte-identical output leaves the terminal in
// an identical state — the same color in force, the same cell under the
// cursor — so a redraw may resume in the middle of a highlighted run without
// re-stating the run. The count of characters is what turns the byte offset
// back into a place on the screen, and escape sequences do not contribute to
// it because they occupy no cells.
//
// The scan is token by token, never byte by byte: a cut inside a multi-byte
// character or inside an escape sequence would name a screen position that
// does not exist.
func sharedPrefix(old, cur string) (bytes, chars int) {
	i, n := 0, 0
	for i < len(old) && i < len(cur) {
		escape, size := nextToken(cur, i)
		if i+size > len(old) || old[i:i+size] != cur[i:i+size] {
			return i, n
		}
		if !escape {
			n++
		}
		i += size
	}
	return i, n
}

// styleInForce is the attributes a redraw has to re-state before it may resume
// writing at at: everything [editor.styled] switched on before that byte and
// has not switched off again, or nothing where no run is open there.
//
// [sharedPrefix] answers where two draws stop agreeing, which is a *cell* the
// cursor can be moved to. It is not a *state* the terminal is in. The previous
// draw wrote the whole line and then walked the cursor back over it, and a
// cursor move carries no attributes — so what is in force is whatever its last
// byte left, which is the terminal's default, because styled closes every run
// it opens. A shared prefix ending inside a colored run therefore names a
// position where the run is over as far as the terminal is concerned, and the
// bytes after it were written expecting it to be in force.
//
// Measured 2026-09-13 through a pseudo-terminal at 80 columns, typing
// `echo "one two` into the built shell under `-highlight`, which is the
// UnclosedQuote every interactive session gets:
//
//	before   echo \e[31m"\e[0m o\e[0m n\e[0m e\e[0m …
//	after    echo \e[31m"\e[0m \e[31mo\e[0m \e[31mn\e[0m \e[31me\e[0m …
//
// Read as writes: the keystroke that typed the quotation wrote `\e[31m"\e[0m`,
// and the keystroke after it wrote `o\e[0m` — an `o` belonging to the red run,
// drawn with the terminal already back at its default. On screen the quotation
// mark was red and the whole unclosed word after it was plain. Not a corner:
// driver/interactive.go installs a highlighter for every interactive shell and
// a real terminal always has a width, so this is the path a person is on, and
// with a highlighter that colors words it was every word — only the first
// character of each kept its color (#2627).
//
// **Re-stating the style rather than redrawing the run.** The other repair is
// to walk the resume point back to where no run is open and write the run
// again from its opening sequence. That is correct too, and it costs the run:
// measured the same way, typing the eight characters of `"one two` wrote 130
// bytes that way against 80 for this, and the eighth keystroke on its own was
// 21 bytes against 10 — a gap that grows with the word, where this one does
// not. Re-stating is also the smaller change: the resume point does not move,
// so nothing counted against it has to be recounted.
//
// The scan is token by token for the reason sharedPrefix's is: a cut inside an
// escape sequence would name a state that does not exist. Everything since the
// last reset is kept rather than only the last sequence, because a terminal
// composes them — a highlighter emitting a color and then a weight has both
// in force, and re-stating only the weight would resume in the wrong color.
func styleInForce(cur string, at int) string {
	var open []string
	for i := 0; i < at; {
		escape, size := nextToken(cur, i)
		if escape {
			if tok := cur[i : i+size]; tok == highlightReset {
				open = open[:0]
			} else {
				open = append(open, tok)
			}
		}
		i += size
	}
	return strings.Join(open, "")
}

// nextToken is the length of the thing at s[i] — one escape sequence, or one
// character — and whether it was an escape sequence.
//
// The same scan [displayWidth] makes, kept beside its caller rather than
// shared with it: that one is adding up cells and this one is cutting a
// string, and folding them would give one of them the other's edge cases.
func nextToken(s string, i int) (escape bool, size int) {
	if s[i] != esc {
		_, n := utf8.DecodeRuneInString(s[i:])
		return false, n
	}
	j := i + 1
	if j < len(s) && s[j] == '[' {
		j++
		for j < len(s) && (s[j] < '@' || s[j] > '~') {
			j++
		}
	}
	if j < len(s) {
		j++
	}
	return true, j - i
}

// moveCursor writes the shortest way of getting from one cell to another.
//
// Rows first and then the column, because a vertical move leaves the column
// alone and the two counts are independent. Within a row it is whichever of a
// relative move and a carriage return is shorter — which is not a
// micro-optimization but the common case: the cursor walking left through a
// line it is editing is one `\e[nD` and never a return and a walk back out.
func moveCursor(b *strings.Builder, fromRow, fromCol, toRow, toCol int) {
	if fromRow == toRow && fromCol == toCol {
		return
	}
	if toRow != fromRow {
		b.WriteString("\x1b[")
		if toRow < fromRow {
			b.WriteString(itoa(fromRow - toRow))
			b.WriteString("A")
		} else {
			b.WriteString(itoa(toRow - fromRow))
			b.WriteString("B")
		}
		// A vertical move keeps the column, so the horizontal move that
		// follows starts from the column the cursor was already on.
	}
	writeColumn(b, fromCol, toCol)
}

// writeColumn moves along one row.
func writeColumn(b *strings.Builder, fromCol, toCol int) {
	switch {
	case toCol == fromCol:
	case toCol > fromCol:
		b.WriteString("\x1b[")
		b.WriteString(itoa(toCol - fromCol))
		b.WriteString("C")
	default:
		// Three ways to go left, and the shortest of them wins. A backspace
		// is one byte and moves one column, so a short walk back — which is
		// what a cursor key and a delete are — beats the four bytes of the
		// sequence that says the same thing. It is what zsh emits for the
		// same move.
		steps := fromCol - toCol
		back := len("\x1b[") + len(itoa(steps)) + 1
		ret := 1
		if toCol > 0 {
			ret = 1 + len("\x1b[") + len(itoa(toCol)) + 1
		}
		switch {
		case steps < back && steps <= ret:
			b.WriteString(strings.Repeat("\b", steps))
		case ret <= back:
			b.WriteString("\r")
			if toCol > 0 {
				b.WriteString("\x1b[")
				b.WriteString(itoa(toCol))
				b.WriteString("C")
			}
		default:
			b.WriteString("\x1b[")
			b.WriteString(itoa(steps))
			b.WriteString("D")
		}
	}
}

// placeAt is where one position in the line lands on the screen. See place,
// which answers the same question and the end of the line's with it.
func placeAt(promptWidth int, line []rune, pos, cols int) (row, col int) {
	row, col, _, _ = place(promptWidth, line, pos, cols)
	return row, col
}

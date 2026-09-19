// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "strings"

// The right prompt: text drawn against the right-hand edge of the row the
// line is being typed on.
//
// No dialect in this tree has ever drawn one. `RPROMPT`/`RPS1` appears in
// dialect/zsh/promptnames.go as the name-aliasing measurement and no drawing
// code reads it, so this is a capability addition rather than a theme detail
// — bash, ksh, dash and ash get a right prompt they have never had, and the
// zsh dialect gets to *name* the one the substrate draws.
//
// **What it does was measured rather than decided**, because zsh is the only
// column in the panel that has one. zsh 5.9.2 through a pseudo-terminal,
// 2026-09-19; docs/spec/prompt-theme.md carries the run and the bytes. Three
// facts came out of it and all three are implemented here:
//
//   - It is placed against the **edge and not the line**, ending **one cell
//     short** of the edge. At 40 columns a five-cell right prompt fills 35
//     through 39 and column 40 stays blank.
//   - It is drawn while the line is at least one blank cell short of it, so
//     the widest line that keeps it is `cols - right - 2` — checked at six
//     width and right-width pairs and equal to the formula at every one. A
//     terminal too narrow for both sides is that rule and not a special case.
//   - It is hidden when the line grows into it and drawn again when the line
//     shrinks back.
//
// One thing zsh does that this does not: it **leaves the right prompt in
// scrollback**. Accepting a line that is showing one writes the bracketed
// paste sequence and a newline, and nothing erases the row. That is the one
// deliberate departure, and the reason is that the row was laid out for the
// width the terminal had at the time — a resize smears it, and it is
// decoration attached to output people actually read. See rightPromptErase.

// rightFits reports whether a right prompt of rightCells can be drawn beside
// a line of lineCells under a prompt of promptCells.
//
// The two spare cells are the measurement: one blank column at the right-hand
// edge, which the right prompt never occupies, and one blank column between
// the end of the line and the start of the right prompt. A right prompt of no
// width is not drawn at all, which keeps every prompt that has no right half
// on exactly the path it was on before this existed.
func rightFits(promptCells, lineCells, rightCells, cols int) bool {
	if rightCells <= 0 || cols <= 0 {
		// No right half, or no width to place one in. A width of zero is not
		// a width of eighty: placing against an edge nobody knows is how a
		// frame ends up wrapped.
		return false
	}
	return promptCells+lineCells+rightCells+2 <= cols
}

// rightPromptAt is where a right prompt starts, counted in columns from the
// left-hand edge with the first column numbered **zero**.
//
// Against the edge rather than after the line, which is what makes it stay
// still while the line grows under it. The extra one is the blank column at
// the edge: measured, zsh at 40 columns puts a five-cell right prompt in
// columns 35 through 39 and leaves column 40 empty, which is columns 34
// through 38 counted from zero.
//
// The off-by-one here is the whole of the difference between drawing against
// the edge and drawing *over* it, and a terminal with automatic margins turns
// the second into a wrap — so the table in TestTheRightPromptSitsWhereZshPutsIt
// is the measurement rather than a rounding of it.
func rightPromptAt(rightCells, cols int) int { return cols - rightCells - 1 }

// writeRightPrompt draws the right prompt and returns the cursor to the start
// of the row.
//
// Called with the cursor immediately after the line, which is where the draw
// that wrote the line leaves it. It writes a forward move to the right
// prompt's column, the prompt, and a carriage return — so a caller placing
// the cursor afterwards counts from column zero, which is what every caller
// here already does.
//
// Nothing at all is written when it does not fit, and that is what erases it:
// the whole-line draw clears to the end of the screen before it writes the
// prompt, so a right prompt that no longer fits is gone by the time this
// declines to write one.
func writeRightPrompt(b *strings.Builder, prompt drawnPrompt, lineCells, cols int) bool {
	if !rightFits(prompt.cells, lineCells, prompt.rightCells, cols) {
		return false
	}
	forward := rightPromptAt(prompt.rightCells, cols) - (prompt.cells + lineCells)
	if forward > 0 {
		b.WriteString("\x1b[")
		b.WriteString(itoa(forward))
		b.WriteString("C")
	}
	b.WriteString(prompt.right)
	b.WriteString("\r")
	return true
}

// rightPromptErase takes a drawn right prompt off the row, leaving the cursor
// where it found it.
//
// This is the departure from zsh. A right prompt left on an accepted line's
// row is a fragment: the row was laid out for the width the terminal had at
// the time, so a resize smears it, and it is decoration attached to output
// somebody is going to read. So the row is cleared from the end of the line
// rightwards before the line is finished.
//
// From the end of the *line* rather than from the right prompt's own column,
// because a single erase-to-end-of-row takes the gap and the prompt together
// and there is nothing between them to keep.
func rightPromptErase(b *strings.Builder, prompt drawnPrompt, lineCells, cols, curCol int) {
	if !rightFits(prompt.cells, lineCells, prompt.rightCells, cols) {
		return
	}
	b.WriteString("\r")
	if end := prompt.cells + lineCells; end > 0 {
		b.WriteString("\x1b[")
		b.WriteString(itoa(end))
		b.WriteString("C")
	}
	b.WriteString("\x1b[K")
	// Back where the caller had it. An erase that moved the cursor would
	// leave whatever writes next — the unfinished mark, the newline — in the
	// wrong column, which is the failure this sits one line away from.
	b.WriteString("\r")
	if curCol > 0 {
		b.WriteString("\x1b[")
		b.WriteString(itoa(curCol))
		b.WriteString("C")
	}
}

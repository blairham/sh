// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "strings"

// Drawing the line on a terminal that cannot move the cursor right.
//
// A `dumb` terminal, or one the terminfo database has no description for, has
// a carriage return and a backspace and nothing else to move with, and the
// redraw in repaint.go speaks ANSI cursor movement to every terminal. bash
// draws there the way its line editor draws on any terminal without the
// capability: backspaces to go left, and the prompt and the line written again
// from a carriage return to go right. Measured 2026-10-07 through a
// pseudo-terminal under `TERM=dumb`, bash 5.3.20 with `PS1='P> '`, the bytes
// each key wrote:
//
//	echo abcdef, Left×3, X       \b\b\b, then Xdef\b\b\b
//	… Left×3, Backspace           \bdef \b\b\b\b
//	… Left×3, ^K                  ' ' ×3, then \b×3
//	echo abc, ^W                  \b\b\b, ' ' ×3, \b\b\b
//	echo abc, ^A                  \r\rP>                 (8 back costs more)
//	… ^A, #                       #echo abc, then \r\rP> #
//	echo abcdefghij, ^A, ^F       \rP> e                 (every move right)
//	echo three, Up (echo one)     \b×5, one, ' ' ×2, \b\b
//
// So a move left is backspaces unless two carriage returns and the prompt and
// the line up to the place are fewer bytes, and a move right is always the
// line written again from one. What changed is written from the first
// character that differs, and a line that got shorter has its old tail covered
// with spaces. No escape sequence is written at all (#6314). See
// EditorStyle.DrawsWithoutCursorMotion.

// cannotMoveTheCursor reports whether the terminal `$TERM` names has no way
// to move the cursor right: no description at all, or one without `cuf1` and
// `cuf`. Cached for the `$TERM` it was asked about, since it reads a file.
func (s Shell) cannotMoveTheCursor() bool {
	if !s.Editor.DrawsWithoutCursorMotion || s.Runner == nil {
		return false
	}
	env := func(name string) string {
		v, _ := s.Runner.GetVar(name)
		return v
	}
	key := env("TERM") + "\x00" + env("TERMINFO") + "\x00" + env("TERMINFO_DIRS")
	c := s.counted()
	if c.motionAsked && c.motionKey == key {
		return c.noMotion
	}
	no := true
	for _, cap := range TerminalCapabilities(env) {
		if cap.Terminfo == "cuf1" || cap.Terminfo == "cuf" {
			no = false
			break
		}
	}
	c.motionAsked, c.motionKey, c.noMotion = true, key, no
	return no
}

// redrawWithoutMotion is redraw for a terminal that cannot move the cursor
// right. See the comment at the top of this file.
func (e *editor) redrawWithoutMotion(prompt drawnPrompt, cols int) {
	line := e.displayed()
	var b strings.Builder
	d := e.drawn
	if !d.valid || !d.plain || d.prompt != prompt.text {
		// Nothing known about the row, so all of it is written again.
		b.WriteString("\r")
		b.WriteString(prompt.text)
		b.WriteString(e.spell(hideControls(string(line)), prompt.cells, 0))
		e.backTo(&b, prompt, line, prompt.cells+cells(line), e.pos)
		e.finishWithoutMotion(&b, prompt, line, cols)
		return
	}
	old := []rune(d.styled)
	at := 0
	for at < len(old) && at < len(line) && old[at] == line[at] {
		at++
	}
	col := d.col
	if at < len(old) || at < len(line) {
		col = e.goTo(&b, prompt, line, col, at)
		b.WriteString(e.spell(hideControls(string(line[at:])), col, 0))
		col = prompt.cells + cells(line)
		if gone := cells(old) - cells(line); gone > 0 {
			b.WriteString(strings.Repeat(" ", gone))
			col += gone
		}
	}
	e.goTo(&b, prompt, line, col, e.pos)
	e.finishWithoutMotion(&b, prompt, line, cols)
}

// goTo moves from column col to the place of line[pos] and answers the
// column it is then in: left by backspaces or from two carriage returns,
// whichever is shorter, and right by writing the line again from one.
func (e *editor) goTo(b *strings.Builder, prompt drawnPrompt, line []rune, col, pos int) int {
	to := prompt.cells + cells(line[:pos])
	switch {
	case to < col:
		e.backTo(b, prompt, line, col, pos)
	case to > col:
		b.WriteString("\r")
		b.WriteString(prompt.text)
		b.WriteString(e.spell(hideControls(string(line[:pos])), prompt.cells, 0))
	}
	return to
}

// backTo moves left from column col to the place of line[pos].
func (e *editor) backTo(b *strings.Builder, prompt drawnPrompt, line []rune, col, pos int) {
	to := prompt.cells + cells(line[:pos])
	steps := col - to
	if steps <= 0 {
		return
	}
	again := e.spell(hideControls(string(line[:pos])), prompt.cells, 0)
	if steps <= 2+len(prompt.text)+len(again) {
		b.WriteString(strings.Repeat("\b", steps))
		return
	}
	b.WriteString("\r\r")
	b.WriteString(prompt.text)
	b.WriteString(again)
}

// finishWithoutMotion writes what was built and records the row as drawn.
func (e *editor) finishWithoutMotion(b *strings.Builder, prompt drawnPrompt, line []rune, cols int) {
	e.row = 0
	if b.Len() > 0 {
		e.write(b.String())
	}
	end := prompt.cells + cells(line)
	e.drawn = drawnLine{
		valid:  true,
		plain:  true,
		styled: string(line),
		prompt: prompt.text,
		cells:  prompt.cells,
		cols:   cols,
		col:    prompt.cells + cells(line[:e.pos]),
		endCol: end,
	}
}

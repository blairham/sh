// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"sort"
	"strings"
)

// Sending the difference rather than the line.
//
// A redraw used to rewrite everything: return to the start of the prompt's
// last row, erase to the end of the screen, and write the prompt and the whole
// styled line again. That is correct, and it costs the line for every
// character typed — measured at #2546, eighteen times the bytes zsh sends for
// the same keystroke, because zsh repaints what changed.
//
// The cost is not the characters. It is the prompt, re-sent each time, and
// every run's escape sequence re-sent with it: on a themed prompt the typed
// character is a rounding error beside them. That is invisible on a local
// terminal and is exactly what is felt over ssh or inside a multiplexer,
// which is where a line editor's latency is noticed at all.
//
// So the editor keeps what it believes is on the screen — the runs it drew and
// where it left the cursor — and the next draw writes only from the first
// place the two disagree. The belief is the whole of the risk, so it is held
// to be false by default: every write that is not a redraw clears it (see
// editor.write), and a draw whose prompt has changed under it does not use it.
// Anything the difference cannot express falls back to drawing the line whole,
// which is always correct.

// styledRun is a stretch of the line drawn with one style. An empty style is
// the line's own color, written with no sequence around it.
type styledRun struct {
	style string
	text  []rune
}

// styledRuns is the line split into the stretches it is drawn as.
//
// The same decisions styled() makes, in pieces rather than as one string,
// because sending a difference means knowing where one stretch ends and the
// next begins. The two must agree byte for byte — writeRunsFrom(runs, 0) is
// styled() — and TestTheRunsAndTheStringAgree is the row that holds them
// together.
func (e *editor) styledRuns() []styledRun {
	line := string(e.line)
	if e.highlighter == nil {
		return e.plainRuns()
	}
	runs := e.highlighter.Highlight(line)
	if len(runs) == 0 {
		return e.plainRuns()
	}
	runs = append([]Highlight(nil), runs...)
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].Start < runs[j].Start })

	var out []styledRun
	at := 0
	for _, r := range runs {
		if r.Start < at || r.End <= r.Start || r.End > len(line) || r.Style == "" {
			continue
		}
		if r.Start > at {
			out = append(out, styledRun{text: []rune(line[at:r.Start])})
		}
		out = append(out, styledRun{style: r.Style, text: []rune(line[r.Start:r.End])})
		at = r.End
	}
	if at == 0 {
		// Nothing applied, so nothing was split.
		return e.plainRuns()
	}
	if at < len(line) {
		out = append(out, styledRun{text: []rune(line[at:])})
	}
	return out
}

// plainRuns is the whole line in one unstyled run.
//
// A copy of the line and not the line itself: what this returns is kept until
// the next draw, and e.line is edited in place by the next keystroke. Holding
// the slice would make the record of what is on the screen change whenever the
// line did, which is a difference that always comes out empty.
func (e *editor) plainRuns() []styledRun {
	if len(e.line) == 0 {
		return nil
	}
	return []styledRun{{text: append([]rune(nil), e.line...)}}
}

// runeLen is how many of the line's characters the runs cover.
func runeLen(runs []styledRun) int {
	n := 0
	for _, r := range runs {
		n += len(r.text)
	}
	return n
}

// commonRunePrefix is how many leading characters two drawings share, counting
// a character as shared only when its style is shared too.
//
// Per character rather than per run, because the same drawing can be split
// into runs two ways — a highlighter that colors a growing word returns one
// run where it returned two — and a split that differs is not a screen that
// differs.
func commonRunePrefix(a, b []styledRun) int {
	n, ai, ar, bi, br := 0, 0, 0, 0, 0
	for {
		for ai < len(a) && ar == len(a[ai].text) {
			ai, ar = ai+1, 0
		}
		for bi < len(b) && br == len(b[bi].text) {
			bi, br = bi+1, 0
		}
		if ai >= len(a) || bi >= len(b) {
			return n
		}
		if a[ai].style != b[bi].style || a[ai].text[ar] != b[bi].text[br] {
			return n
		}
		n, ar, br = n+1, ar+1, br+1
	}
}

// writeRunsFrom writes the drawing from the given character onwards.
//
// A run the split lands inside has its style written again for the remainder.
// That is not redundant: the sequence was sent before the split and the
// terminal is still in it, but the run's closing reset has not been sent, so
// the tail has to say what it is drawn in for the reset at its end to be true.
func writeRunsFrom(b *strings.Builder, runs []styledRun, from int) {
	at := 0
	for _, r := range runs {
		end := at + len(r.text)
		if end <= from {
			at = end
			continue
		}
		start := 0
		if from > at {
			start = from - at
		}
		if r.style == "" {
			b.WriteString(string(r.text[start:]))
		} else {
			b.WriteString(r.style)
			b.WriteString(string(r.text[start:]))
			b.WriteString(highlightReset)
		}
		at = end
	}
}

// moveCursor writes the shortest move from one cell to another.
//
// Relative moves rather than a carriage return and a count: `\r` costs a byte
// and then the whole column has to be counted back, which on a themed prompt
// is most of what the move costs. A keystroke at the end of a line moves
// nowhere at all and writes nothing.
func moveCursor(b *strings.Builder, fromRow, fromCol, toRow, toCol int) {
	switch {
	case toRow < fromRow:
		b.WriteString("\x1b[")
		b.WriteString(itoa(fromRow - toRow))
		b.WriteString("A")
	case toRow > fromRow:
		b.WriteString("\x1b[")
		b.WriteString(itoa(toRow - fromRow))
		b.WriteString("B")
	}
	switch {
	case toCol > fromCol:
		b.WriteString("\x1b[")
		b.WriteString(itoa(toCol - fromCol))
		b.WriteString("C")
	case toCol < fromCol:
		b.WriteString("\x1b[")
		b.WriteString(itoa(fromCol - toCol))
		b.WriteString("D")
	}
}

// rememberDrawn records what the screen now shows, so the next draw can send a
// difference against it. Called after the write, because write is what clears
// the belief.
func (e *editor) rememberDrawn(prompt drawnPrompt, runs []styledRun, row, col int) {
	if e.neverRemember {
		// The tests' way of asking for the draw this replaced, so that the
		// two can be held against each other on the same keystrokes. Never
		// set outside a test.
		e.row, e.col = row, col
		return
	}
	e.drawn = runs
	e.drawnPromptText = prompt.text
	e.row, e.col = row, col
	e.drawnValid = true
}

// forget says the screen is no longer what the editor believes, so the next
// draw writes the line whole.
func (e *editor) forget() { e.drawnValid = false }

// redrawDifference draws the line by sending only what changed, and answers
// whether it could.
//
// It declines rather than guesses. The belief has to be current, the prompt
// has to be the one it was formed against — its text alone, because a prompt's
// width is derived from its text, so a second check on the width could never
// fail where the first had passed — and the runs have to account for
// exactly the characters in the line — a highlighter returning a byte offset
// inside a character would otherwise put every column after it out by one, and
// a line drawn at the wrong column is worse than a line drawn twice.
func (e *editor) redrawDifference(prompt drawnPrompt, runs []styledRun, cols int) bool {
	if !e.drawnValid || e.drawnPromptText != prompt.text {
		return false
	}
	newLen := runeLen(runs)
	if newLen != len(e.line) {
		return false
	}
	split := commonRunePrefix(e.drawn, runs)
	oldLen := runeLen(e.drawn)

	var b strings.Builder
	splitRow, splitCol, _, _ := place(prompt.cells, e.line[:split], split, cols)
	moveCursor(&b, e.row, e.col, splitRow, splitCol)
	if oldLen > split {
		// What the old drawing had past this point has to go, and it may be
		// several rows of it — the same reason the whole-line draw erases to
		// the end of the screen rather than the end of the row.
		b.WriteString("\x1b[J")
	}

	row, col := splitRow, splitCol
	if split < newLen {
		writeRunsFrom(&b, runs, split)
		_, _, row, col = place(prompt.cells, e.line, e.pos, cols)
	}
	if col == cols {
		// The drawing ends exactly at the right-hand edge, where a terminal
		// holds the wrap until there is something to put on the next row. A
		// space forces it and the carriage return takes the space back.
		b.WriteString(" \r")
		row, col = row+1, 0
	}

	curRow, curCol, _, _ := place(prompt.cells, e.line, e.pos, cols)
	if curCol == cols {
		curRow, curCol = curRow+1, 0
	}
	moveCursor(&b, row, col, curRow, curCol)

	e.write(b.String())
	e.rememberDrawn(prompt, runs, curRow, curCol)
	return true
}

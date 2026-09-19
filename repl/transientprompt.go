// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "strings"

// TransientPrompt is asked, as a line is accepted, what to leave behind in
// its place.
//
// An empty answer leaves the prompt as it was drawn, which is what "off"
// means and what a shell that sets nothing gets. Any other answer replaces
// the whole prompt — every row of it — for the copy that stays in scrollback.
//
// **The policy lives above this package, and that is deliberate.** The
// settings a theme offers are `always`, `off` and `same-dir`, and `same-dir`
// has to know whether the previous command changed directory: shell state,
// which `repl` has no business reading. So `repl` asks a question and the
// caller answers it, the way a PromptProvider is a code path the binary
// composes rather than a table this package interprets. It keeps this
// package free of any dialect, which is the property the theme work is
// required to preserve.
//
// It is called once per accepted line, after the line is on the screen and
// before anything is written past it.
type TransientPrompt func() string

// leadRows is how many screen rows a prompt's leading text occupies.
//
// One per newline, because lead is everything up to and including the
// prompt's last newline — so the count of newlines is the count of rows above
// the row the line sits on. Escape sequences in there occupy no cells and no
// rows, which is why this counts newlines rather than measuring width.
//
// A leading row wider than the terminal wraps and takes two, and this does
// not know that. It is the same limit the draw has: editor.readLine writes
// lead once and never rewrites it, so nothing in this file is the first place
// that arithmetic would have to change.
func leadRows(lead string) int { return strings.Count(lead, "\n") }

// trimPrompt repaints the accepted line under a shorter prompt and reports
// the prompt that is now on the screen.
//
// Returning the new prompt is the whole interface: toLastRow counts rows from
// the prompt's width, and it can only count correctly if the width it is
// given is the one the screen is actually wearing. Handing back the trimmed
// prompt is what keeps the step after this one honest.
//
// The repaint has to reach further up than a redraw does. redraw returns with
// `\r`, goes up as far as the line came down, and rewrites from the prompt's
// *last* row — the rows above it are written once and left alone, which is
// what keeps a two-row prompt from laddering. A trim is the case those rows
// are not left alone in: the whole point is that they go away, so this goes
// up past them as well before erasing to the end of the screen.
func (e *editor) trimPrompt(prompt drawnPrompt) drawnPrompt {
	if e.transient == nil {
		return prompt
	}
	text := e.transient()
	if text == "" {
		return prompt
	}
	cols := e.cols()
	if cols <= 0 {
		// No width, so none of the arithmetic below can be done and a partial
		// attempt would leave the screen worse than the untrimmed prompt
		// does. The prompt stays as drawn: a missing feature rather than a
		// corrupted line.
		return prompt
	}

	short := drawnPrompt{text: text, cells: displayWidth(text)}

	var b strings.Builder
	b.WriteString("\r")
	if up := e.row + leadRows(prompt.lead); up > 0 {
		b.WriteString("\x1b[")
		b.WriteString(itoa(up))
		b.WriteString("A")
	}
	// To the end of the screen, not the end of the row: what is being
	// replaced is every row the old prompt and the line occupied.
	b.WriteString("\x1b[J")
	b.WriteString(text)
	b.WriteString(onScreen(e.styled()))

	// The cursor is at the end of the line now rather than at the editing
	// position, because the whole line was just written and nothing moved
	// back over it. Recording the row it ended on is what lets the ordinary
	// toLastRow that follows do the right thing — which, from the last row,
	// is nothing.
	_, _, endRow, endCol := place(short.cells, e.line, len(e.line), cols)
	if endCol == cols {
		// The line ends exactly at the right-hand edge and the terminal has
		// not wrapped yet, so the cursor is still on the old row. A space
		// makes it wrap and the carriage return undoes the space — the same
		// bargain redraw makes, for the same reason.
		b.WriteString(" \r")
		endRow++
	}
	e.write(b.String())
	e.row = endRow
	return short
}

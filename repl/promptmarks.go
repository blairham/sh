// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "strings"

// Where a rendered prompt says "none of this is a column".
//
// FieldNonPrintingStart and FieldNonPrintingEnd draw these two characters, and
// nothing else in the shell writes them: they survive expansion untouched,
// they are what the editor counts by, and they are taken back out again before
// a byte reaches the terminal. A marker that reached the screen would be a
// control character nobody asked for.
//
// SOH and STX rather than something of our own invention, because bash's are
// the same two: measured through a pty, `PS1=$'\001\033[31m\002X'` drew the
// color and the X with neither marker byte on the wire, exactly as `\[` and
// `\]` do. zsh prints both bytes instead, so a zsh prompt holding a literal
// SOH loses it here — invisibly, since a terminal draws it in no cells either
// way, which is the whole of the divergence.
const (
	markStart = "\x01"
	markEnd   = "\x02"
)

// markCell brackets what fills exactly one column whatever its bytes are:
// zsh's `%G`, and a magic-cookie terminal's standout sequence, which the
// terminal stores as a cell of its own. The bytes between two of them are
// written and the pair is counted as one cell, inside the non-printing
// markers or out of them. ETX, beside the two above, for the same reason
// they are what they are.
const markCell = "\x03"

// drawnPrompt is a rendered prompt as the editor uses it: the bytes to write,
// and how many cells they take on the screen.
//
// The two are not the same question and cannot be asked separately later. Once
// the markers are gone the width is unknowable — `\e[31m` and `]0;title\a` are
// bytes like any others — and while they are still there the string cannot be
// written. So they are answered together, once, and carried as a pair.
type drawnPrompt struct {
	// lead is everything up to and including the prompt's last newline, and
	// text is the line after it — the part the cursor sits on.
	//
	// **A prompt with a newline in it is drawn in two pieces, and that is the
	// whole reason this is not one string.** The leading rows are written once
	// when the prompt is first drawn; every redraw after that rewrites only
	// the last row, because that is the row the line is on and the only one a
	// keystroke can change.
	//
	// Writing the whole prompt on every redraw is what this shell did, and it
	// is what a two-row prompt made visible: a fresh copy of the upper row was
	// pushed onto the screen at each keystroke, so pressing Up left a ladder
	// of prompts behind it (#2467). The `\r` a redraw begins with returns to
	// the start of the row the cursor is on, which is the *last* row — so the
	// upper rows were never where the rewriting began, and re-emitting them
	// could only ever add to the screen.
	//
	// Empty lead is the ordinary one-row prompt and costs nothing.
	lead string
	text string

	// cells is how wide text is — the last row alone, not the whole prompt.
	// It is where the line starts on its row, which is what every placement
	// calculation needs; counting the upper rows would put the wrap, the
	// cursor column and the search's arithmetic out by the width of them.
	cells int

	// right is the prompt drawn against the right-hand edge of the row being
	// typed on, and rightCells is how wide it is. Both zero for every prompt
	// that has no right half, which is every prompt this shell drew before
	// rightprompt.go existed.
	//
	// Measured and counted here for the reason text and cells are: once the
	// markers are gone the width is unknowable, and while they are there the
	// string cannot be written.
	right      string
	rightCells int

	// rightIndent is how many columns the right prompt keeps clear of the
	// right-hand edge — zsh's `ZLE_RPROMPT_INDENT`, which is 1 unless a
	// session says otherwise. Set wherever right is, by setRight; a right
	// prompt with the zero value here is drawn into the last column, which a
	// terminal with automatic margins is entitled to wrap.
	rightIndent int

	// regions is how many non-printing regions the last row was written
	// with, and counted how many counted columns. See rowView.promptCost.
	regions, counted int
}

// setRight measures a rendered right prompt into p.
//
// The same treatment the left one has and for the same reason — a right
// prompt is mostly escapes, and the editor places it by its *cells*. It has
// no rows of its own: it is drawn on the row being typed on or it is not
// drawn, so only the last row of what was rendered is kept.
func (p *drawnPrompt) setRight(rendered string, indent int) {
	measured := drawPrompt(rendered)
	p.right, p.rightCells, p.rightIndent = measured.text, measured.cells, indent
}

// drawPrompt takes the markers out and counts what is left.
//
// Everything between a start and an end is dropped from the count and kept in
// the text. Outside them the ordinary reckoning stands, which already skips an
// escape sequence — so a prompt that colors itself without saying so is still
// measured correctly, and a prompt that says so is measured correctly whatever
// it holds. The second is the case that needs the markers: a terminal title
// written `\[\e]0;\w\a\]` is not an escape sequence displayWidth knows, and
// counting it charges the prompt for every letter of the title.
//
// An unmatched marker is not an error and does not eat the rest of the line: a
// start with no end hides what follows it — there is nothing else it could
// mean — and an end with no start is dropped. bash writes that stray end byte
// to the terminal instead, measured, which is a control character on the
// screen for a prompt that said nothing about one.
func drawPrompt(rendered string) drawnPrompt {
	if !strings.ContainsAny(rendered, markStart+markEnd+markCell) {
		// The common case by a long way, and it copies nothing.
		return splitRows(rendered)
	}
	var text, shown strings.Builder
	hidden := false
	// Byte by byte: both markers are ASCII, and every byte of a longer
	// character has its high bit set, so nothing here can split a rune.
	for i := 0; i < len(rendered); i++ {
		switch c := rendered[i]; c {
		case markCell[0]:
			end := strings.IndexByte(rendered[i+1:], markCell[0])
			if end < 0 {
				end = len(rendered) - i - 1
			}
			text.WriteString(rendered[i+1 : i+1+end])
			shown.WriteByte(' ')
			i += end + 1
		case markStart[0]:
			hidden = true
		case markEnd[0]:
			hidden = false
		default:
			text.WriteByte(c)
			if !hidden {
				shown.WriteByte(c)
			}
		}
	}
	// The rows are split on the text with the markers gone, and the width is
	// taken from the shown text's last row — the two are split separately
	// because a marker may sit on either side of the newline and neither
	// string can be measured from the other.
	p := splitRows(text.String())
	p.cells = displayWidth(lastRow(shown.String()))
	p.regions = strings.Count(lastRow(rendered), markStart)
	p.counted = strings.Count(lastRow(rendered), markCell) / 2
	return p
}

// splitRows takes a rendered prompt apart at its last newline.
//
// See drawnPrompt, which carries why the two halves are drawn at different
// times. A prompt with no newline is all text and no lead, which is the
// ordinary case and the one this must not make more expensive.
func splitRows(rendered string) drawnPrompt {
	last := strings.LastIndexByte(rendered, '\n')
	if last < 0 {
		return drawnPrompt{text: rendered, cells: displayWidth(rendered)}
	}
	text := rendered[last+1:]
	return drawnPrompt{lead: rendered[:last+1], text: text, cells: displayWidth(text)}
}

// lastRow is the part of a string after its last newline.
func lastRow(s string) string {
	if last := strings.LastIndexByte(s, '\n'); last >= 0 {
		return s[last+1:]
	}
	return s
}

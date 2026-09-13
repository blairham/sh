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
	if !strings.ContainsAny(rendered, markStart+markEnd) {
		// The common case by a long way, and it copies nothing.
		return splitRows(rendered)
	}
	var text, shown strings.Builder
	hidden := false
	// Byte by byte: both markers are ASCII, and every byte of a longer
	// character has its high bit set, so nothing here can split a rune.
	for i := 0; i < len(rendered); i++ {
		switch c := rendered[i]; c {
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

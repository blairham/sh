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
	text  string
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
		return drawnPrompt{text: rendered, cells: displayWidth(rendered)}
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
	return drawnPrompt{text: text.String(), cells: displayWidth(shown.String())}
}

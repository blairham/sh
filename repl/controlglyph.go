// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"unicode/utf8"
)

// A control character in the line, and how it is drawn (#5972).
//
// The line used to refuse them, because there was no way to draw one: a raw
// byte sent to the terminal moves the cursor or does nothing, and every
// column counted after it is wrong. zsh and bash both keep them — a widget's
// `zle .self-insert` on `^T` puts the byte in the line, and a paste keeps its
// tabs and control characters — and draw them in a way the terminal cannot
// misread. Measured 2026-10-05 through a pseudo-terminal against zsh 5.9.2
// and bash 5.3, a paste of `a\tb\x01c\x1b[31md\x7fe` after a `P> ` prompt:
//
//	zsh   a····b ^A c ^[ [31md ^? e     each caret in standout: \e[7m^A\e[27m
//	bash  a····b ^A c ^[ [31md ^? e     the carets plain
//
// So a control character is a caret and the character 64 places on, in two
// cells — `^?` for delete and `^[` for an escape — and a tab is spaces to the
// next multiple of eight **on the screen's row**, prompt included: after
// `P> ` (three cells) a tab at the start of the line is five spaces, after
// `P> x` four. A tab never wraps: from the last stop of a row it runs to the
// edge, and the next character starts the row below. A caret does wrap
// between its two cells — measured with the line filled to the last column,
// zsh draws the `^` there and the letter at the start of the next row. And
// the cursor counts cells, not characters: `C-b` twice from after `^?e` is
// three backspaces in both shells.
//
// Whether the caret is in standout is the one difference, and it is zsh's
// `zle_highlight` default for its `special` context; see
// EditorStyle.ControlCharacterStyle.

// isControl reports whether a character of the line is drawn by this file:
// everything below a space and delete, except the newline a paste puts in the
// line, which onScreen already spells and which ends a row.
func isControl(r rune) bool {
	return (r < 0x20 && r != '\n') || r == del
}

// hiddenBase is where styled parks the line's control characters until the
// draw spells them. A private-use plane, so a parked character can never be
// read as an escape sequence a highlighter wrote — the reason a literal ESC in
// the line cannot simply be left where it is — and so each is still one
// character to sharedPrefix, which counts the line a character at a time.
const hiddenBase = 0xF0000

// hideControls parks the control characters of a piece of the line.
func hideControls(s string) string {
	if !strings.ContainsFunc(s, isControl) {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if isControl(r) {
			r += hiddenBase
		}
		b.WriteRune(r)
	}
	return b.String()
}

// hidden reports whether a character of a styled line is a parked control
// character, and which.
func hidden(r rune) (rune, bool) {
	if r >= hiddenBase && r <= hiddenBase+del && isControl(r-hiddenBase) {
		return r - hiddenBase, true
	}
	return 0, false
}

// tabWidth is how many cells a tab at col takes on a row cols wide: to the
// next multiple of eight, and no further than the edge. cols of 0 or less is
// a width nobody knows, and the stop alone decides.
func tabWidth(col, cols int) int {
	w := 8 - col%8
	if cols > 0 && col+w > cols {
		w = cols - col
	}
	return w
}

// caret is how a control character is spelled: `^` and the character 64
// places on, which makes delete `^?`.
func caret(r rune) string {
	return "^" + string(r^0x40)
}

// spell is a styled line as the terminal has to be given it, drawn from
// column col of a row cols wide: a newline is a carriage return and a line
// feed (see onScreen), a tab the spaces to its stop, and any other control
// character its caret, between on and off.
//
// Escape sequences pass through, and the ones still in force are said again
// after a caret's off: off turns its one attribute off, and a caret inside a
// run that had the same attribute on — a paste's reverse video — would
// otherwise end that run on the screen. Measured, zsh does the same, writing
// the paste's `\e[7m` again after a caret inside it.
func spell(s string, col, cols int, on, off string) string {
	if !strings.ContainsAny(s, "\n") && !hasHidden(s) {
		return s
	}
	out, _ := spellFrom(s, col, cols, on, off)
	return out
}

// spellFrom is spell, and the column the spelled text ends in.
func spellFrom(s string, col, cols int, on, off string) (string, int) {
	var b strings.Builder
	var open []string
	for i := 0; i < len(s); {
		escape, size := nextToken(s, i)
		tok := s[i : i+size]
		i += size
		if escape {
			if tok == highlightReset {
				open = open[:0]
			} else {
				open = append(open, tok)
			}
			b.WriteString(tok)
			continue
		}
		r, _ := utf8.DecodeRuneInString(tok)
		if r == '\n' {
			b.WriteString("\r\n")
			col = 0
			continue
		}
		if cols > 0 && col >= cols {
			col = 0
		}
		c, ok := hidden(r)
		switch {
		case !ok:
			b.WriteString(tok)
			w := runeWidth(r)
			if cols > 0 && col+w > cols {
				col = 0
			}
			col += w
		case c == '\t':
			w := tabWidth(col, cols)
			b.WriteString(strings.Repeat(" ", w))
			col += w
		default:
			b.WriteString(on)
			b.WriteString(caret(c))
			if off != "" {
				b.WriteString(off)
				for _, o := range open {
					b.WriteString(o)
				}
			}
			col += 2
			if cols > 0 && col > cols {
				col -= cols
			}
		}
	}
	return b.String(), col
}

// hasHidden reports whether a styled line holds a parked control character.
func hasHidden(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool {
		_, ok := hidden(r)
		return ok
	})
}

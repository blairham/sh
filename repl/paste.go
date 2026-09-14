// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// Pasted text, and why a shell has to ask for it.
//
// A terminal cannot tell typing from pasting on its own, and neither can the
// program reading from it: both arrive as bytes on the same descriptor. So the
// terminal offers to say which is which, and it says it only to an application
// that asked — `\e[?2004h` turns the offer on, and from then on pasted text
// arrives wrapped in `\e[200~` and `\e[201~`.
//
// **Not asking is not a neutral choice.** Newlines inside an unbracketed paste
// are keystrokes like any other, so every line of it runs as it arrives: a
// paste of five commands runs five commands, and a paste that was meant to be
// read before it ran has already run. Bracketing is what turns the same paste
// into text in the line, for a person to look at and press Return over — which
// is what every terminal-using shell people paste into does (#2775).
//
// Measured 2026-09-14 through a pseudo-terminal, driving each shell with the
// prompt handed over from outside:
//
//	bash 5.3.3   \e[?2004h before the prompt, \e[?2004l\r after the line
//	zsh 5.9.2    the same two, the first written after the prompt
//	ksh93        neither: the markers are typed into the line as `^[[200~`
//
// The two that ask agree on the bytes and differ only in where in the stream
// the first one goes, which is not something a screen can show — so this is one
// answer per dialect and not two. ksh93 is the disagreement, and it is why
// [EditorStyle.BracketedPaste] exists rather than this being on every dialect's
// path.
const (
	// pasteModeOn asks the terminal to bracket pasted text.
	pasteModeOn = "\x1b[?2004h"

	// pasteModeOff takes the request back, for the run of the command about to
	// be executed. The carriage return is part of what both shells write: the
	// line has just ended, and the cursor goes back to the left margin.
	pasteModeOff = "\x1b[?2004l\r"
)

// The parameters of the two markers, as the control sequence spells them:
// `\e[200~` opens a paste and `\e[201~` closes it. See controlSequence, which
// is what reads them — a paste opens where any other keypad key would be
// recognized, because that is exactly the shape a marker has.
const (
	pasteBegins = 200
	pasteEnds   = 201
)

// pasteEndMarker is the closing marker in full, which is what the read below
// scans for.
const pasteEndMarker = "\x1b[201~"

// insertPaste reads a paste through to its closing marker and puts it in the
// line as text.
//
// The whole of it at once, and drawn once. Both halves matter: inserting rune
// by rune through the typing path would run the completion, the history and the
// undo bookkeeping for every character of somebody's twenty-line function, and
// drawing per rune is what made a 405-byte paste cost 88KB of terminal traffic
// (#1742).
func (e *editor) insertPaste(prompt drawnPrompt) keyRead {
	text, got := e.readPaste()
	if len(text) > 0 {
		at := e.pos
		e.change(false, func() { e.insertAll(text) })
		// The run this put in the line, so that the draw below can mark it as
		// pasted. See styled.
		e.pastedFrom, e.pastedTo = at, at+len(text)
		// Drawn now rather than when the input runs out, which is the rest of
		// #2775: the text landed in the line and nothing appeared until the
		// next keystroke forced a redraw. A paste is one draw whatever else is
		// waiting — the coalescing on the typing path exists to stop a draw
		// *per character*, and there is only one here.
		e.redraw(prompt)
	}
	return got
}

// readPaste reads the bytes up to the closing marker and returns what of them
// belongs in the line.
//
// Through nextByte rather than readByte, because this is text and not a key:
// readByte turns a `\x03` into an abandoned line, and a `^C` a person copied is
// a character of what they copied. There is no cap on the length for the same
// reason — the paste is as long as the paste is.
func (e *editor) readPaste() ([]rune, keyRead) {
	var raw []byte
	var buf [1]byte
	for {
		n, err := e.nextByte(buf[:])
		if err != nil {
			// The input ended mid-paste. What arrived is still text somebody
			// pasted, so it goes in the line and the caller is told the read
			// stopped.
			return pastedRunes(raw), keyStopped
		}
		if n == 0 {
			continue
		}
		raw = append(raw, buf[0])
		if bytes.HasSuffix(raw, []byte(pasteEndMarker)) {
			return pastedRunes(raw[:len(raw)-len(pasteEndMarker)]), keyContinues
		}
	}
}

// pastedRunes is the pasted bytes as characters of the line.
//
// Two rules, and both are measured rather than chosen:
//
// A line ending becomes a newline *in the line*, and does not submit it. That
// is the point of the bracketing. Measured against bash 5.3.3, a paste of
// `echo AAA\recho BBB` draws on two rows and runs neither command until Return
// is pressed, at which point both run; a `\r\n` draws on three, because each of
// the two is a break of its own rather than one line ending spelled twice.
//
// Every other control character is dropped. Real bash keeps them and draws them
// in caret notation — a pasted tab is `^I` on the screen and a tab in the line —
// and this editor has no caret notation: it refuses a typed control character
// for the same reason, that a raw byte in the line is something nobody typed and
// hands the parser a word it cannot have meant. Drawing them as themselves is
// worse than dropping them, because a tab moves the cursor to the next tab stop
// and every column this package counts after it is wrong.
func pastedRunes(raw []byte) []rune {
	out := make([]rune, 0, len(raw))
	for i := 0; i < len(raw); {
		r, size := utf8.DecodeRune(raw[i:])
		i += size
		switch {
		case r == '\r' || r == '\n':
			out = append(out, '\n')
		case r < 0x20 || r == del:
			continue
		case r == utf8.RuneError && size == 1:
			// A byte that is not part of any character. A terminal can deliver
			// one when a paste is cut in the middle of a rune by something
			// upstream, and putting it in the line would put half a character
			// on the screen.
			continue
		default:
			out = append(out, r)
		}
	}
	return out
}

// insertAll puts a run of characters in the line at the cursor, and leaves the
// cursor after them.
func (e *editor) insertAll(rs []rune) {
	e.line = append(e.line, rs...)
	copy(e.line[e.pos+len(rs):], e.line[e.pos:])
	copy(e.line[e.pos:], rs)
	e.pos += len(rs)
}

// forgetPaste stops marking the run the last paste put in the line.
//
// Measured: in bash 5.3.3 and zsh 5.9.2 alike the reverse video comes off on
// the next keystroke, whatever that keystroke is — the line is drawn again
// without it. So this is called for every key before the key acts, next to the
// rest of the bookkeeping about the keystroke before this one, and a paste is
// the only thing that sets it again.
func (e *editor) forgetPaste() { e.pastedFrom, e.pastedTo = 0, 0 }

// onScreen is the line's text as the terminal has to be given it.
//
// A newline in the line — which only a paste puts there — is a line feed and
// nothing else: the terminal is in raw mode, so it moves down a row and leaves
// the cursor in the column it was in, trailing the next row's text off to the
// right. The carriage return is what makes it the start of the row below, which
// is where this package's arithmetic says the next character goes. See place.
//
// The check first because a line with no newline in it is every line anybody
// types, and this is on the draw path.
func onScreen(s string) string {
	if !strings.Contains(s, "\n") {
		return s
	}
	return strings.ReplaceAll(s, "\n", "\r\n")
}

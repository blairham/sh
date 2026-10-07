// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "unicode"

// readline's vi-insert keymap differs from its emacs one beyond the keys it
// types (see EditorStyle.ViInsertTypesTheseKeys) on five more (#6304).
// Measured 2026-10-06 through a pseudo-terminal against bash 5.3.20, `--norc
// -i`, `set -o vi`, `INPUTRC=/dev/null`:
//
//	^N ^P   menu-complete and menu-complete-backward: see
//	        EditorStyle.MenuReturnsToTheWord for the walk
//	^W      vi-unix-word-rubout: see viUnixWordRubout
//	^_      vi-undo: see EditorStyle.ViUndoAsReadline
//	^D      vi-eof-maybe: end of input on an empty line, and the line
//	        accepted, wherever the cursor is, on any other — `echo ab`, Left,
//	        `^D` runs `echo ab`
//
// EditorStyle.ReadlineViInsertKeymap is the switch.

// readlineViInsertKeys are the keys above whose action is a widget.
var readlineViInsertKeys = map[byte]Widget{
	0x0e: WidgetMenuComplete,
	0x10: WidgetMenuCompleteBackward,
	0x17: WidgetViUnixWordRubout,
}

// ReadlineViInsertBindings is readlineViInsertKeys for a dialect's key
// listing.
func ReadlineViInsertBindings() map[string]Widget {
	out := map[string]Widget{}
	for b, w := range readlineViInsertKeys {
		out[string([]byte{b})] = w
	}
	return out
}

// readlineViInsertKey answers a control key in vi insert mode the way
// readline's vi-insert keymap does, and reports whether it did. accept says
// the key accepted the line.
func (e *editor) readlineViInsertKey(c byte, prompt drawnPrompt) (handled, accept bool) {
	if !e.readlineViInsert || !e.viEditing() || e.viCommand {
		return false, false
	}
	if c == ctrlD {
		// vi-eof-maybe: an empty line is the key loop's end of input.
		return len(e.line) > 0, len(e.line) > 0
	}
	w, ok := readlineViInsertKeys[c]
	if !ok {
		return false, false
	}
	b := Binding{Widget: w}
	e.keyBinding = &b
	e.runWidget(b, prompt)
	return true, false
}

// viUnixWordRubout is `^W` in readline's vi insert mode, and reports whether
// it killed anything. Measured 2026-10-06 against bash 5.3.20 with the cursor
// at every place in `aa bb..cc  dd` and in `a.. ..b  .a`:
//
//   - A word is letters and digits — `_` is not one, `é` is — and everything
//     else, blanks and punctuation alike, is the other kind.
//   - At the end of the line, blanks before the cursor go first, and then the
//     run of whichever kind is before them: `aa bb␠␠` loses `bb␠␠`, `aa .;`
//     loses ` .;`.
//   - Elsewhere, the run of the kind before the cursor, back from the cursor:
//     `aa |bb` loses the blank, `a.. ..|b` loses `.. ..`, `c|c` loses a `c` —
//     except that a word before the cursor and something else under it kill
//     nothing at all: `aa| bb` and `bb|..` are left alone, with no bell.
//   - At the start of the line it rings.
//
// It does not stop where the stretch of insert mode began, and two of them in
// a row are two kills rather than one: `^Y` after `aa bb`, `^W`, `^W` puts
// back `aa ` alone.
func (e *editor) viUnixWordRubout() bool {
	if e.pos == 0 {
		return false
	}
	word := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }
	i := e.pos
	if e.pos == len(e.line) {
		for i > 0 && unicode.IsSpace(e.line[i-1]) {
			i--
		}
	} else if word(e.line[i-1]) && !word(e.line[i]) {
		return true
	}
	if i > 0 && word(e.line[i-1]) {
		for i > 0 && word(e.line[i-1]) {
			i--
		}
	} else {
		for i > 0 && !word(e.line[i-1]) {
			i--
		}
	}
	e.killedBefore = false
	e.killTo(i)
	return true
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "strings"

// zsh's vi insert keymap is not its emacs keymap with the escape key changed
// (#6272). Measured 2026-10-06 with `zsh -f -c 'bindkey -M viins'` on zsh
// 5.9.2, and each key through a pseudo-terminal with a two-row prompt and
// `bindkey -v`:
//
//	^A ^B ^E ^F ^K ^N ^O ^P ^S ^T ^Y      self-insert: `ab ^B ^T` runs `ab^B^T`
//	^\ ^] ^^ ^_                           self-insert
//	^D                                    list-choices on a line with
//	                                      something typed — a bell for `echo
//	                                      ab`, nothing deleted; an empty line
//	                                      is still end of input
//	^H ^?                                 vi-backward-delete-char
//	^Q                                    vi-quoted-insert
//	^R                                    redisplay: the whole prompt redrawn
//	^U                                    vi-kill-line
//	^W                                    vi-backward-kill-word
//	^X                                    nothing bound: a bell
//
// The three vi- deletions stop where this stretch of insert mode began. `aa`,
// ESC, `A`, `b`: Backspace deletes the `b` and rings the next time; `^U` kills
// the `b` and the next does nothing, silently; `^W` the same. On a fresh line
// the stretch began at the start. `^W`'s word is vi's — `a.bc` loses `bc`, and
// `ab␠␠` loses all four.
//
// `^C` and `^Z` are listed as self-insert too and are the terminal's, as they
// are in every keymap here; `^G` is list-expand, which this editor has not got,
// and is left doing nothing; `^I`, `^J`, `^L`, `^M`, `^V` and ESC are what
// they are in emacs editing. EditorStyle.ZshViInsertKeymap is the switch, and
// the zero value is the shared table bash's vi insert mode has always had.

// zshViInsertKeys is the table above for the keys whose action is a widget.
var zshViInsertKeys = map[byte]Widget{
	0x01: WidgetSelfInsert, 0x02: WidgetSelfInsert, 0x05: WidgetSelfInsert,
	0x06: WidgetSelfInsert, 0x0b: WidgetSelfInsert, 0x0e: WidgetSelfInsert,
	0x0f: WidgetSelfInsert, 0x10: WidgetSelfInsert, 0x13: WidgetSelfInsert,
	0x14: WidgetSelfInsert, 0x19: WidgetSelfInsert, 0x1c: WidgetSelfInsert,
	0x1d: WidgetSelfInsert, 0x1e: WidgetSelfInsert, 0x1f: WidgetSelfInsert,
	0x08: WidgetViBackwardDeleteChar, 0x7f: WidgetViBackwardDeleteChar,
	0x11: WidgetViQuotedInsert,
	0x12: WidgetRedisplay,
	0x15: WidgetViKillLine,
	0x17: WidgetViBackwardKillWord,
}

// ZshViInsertBindings is zshViInsertKeys for a dialect's key listing, with
// `^D` — which the key loop answers itself, since its empty line is end of
// input — as list-choices.
func ZshViInsertBindings() map[string]Widget {
	out := map[string]Widget{"\x04": WidgetListChoices}
	for b, w := range zshViInsertKeys {
		out[string([]byte{b})] = w
	}
	return out
}

// zshViInsertKey answers a control key in vi insert mode the way zsh's viins
// keymap does, and reports whether it did. A key it leaves is the shared
// table's.
func (e *editor) zshViInsertKey(c byte, prompt drawnPrompt) bool {
	if !e.zshViInsert || !e.viEditing() || e.viCommand {
		return false
	}
	switch c {
	case ctrlD:
		if len(e.line) == 0 {
			return false
		}
		e.listChoices(e.comp, prompt)
		return true
	case ctrlX:
		e.ring()
		return true
	}
	w, ok := zshViInsertKeys[c]
	if !ok {
		return false
	}
	if w == WidgetSelfInsert {
		e.typeCounted(rune(c), 1, prompt)
		return true
	}
	e.runWidget(Binding{Widget: w}, prompt)
	return true
}

// viBackwardDeleteChar is Backspace in vi insert mode: back to where this
// stretch of insert mode began, and a bell there.
func (e *editor) viBackwardDeleteChar() bool {
	if e.pos <= e.viInsertStart {
		return false
	}
	return e.deleteBackward()
}

// viKillLine is `^U` in vi insert mode: everything typed in this stretch of
// insert mode before the cursor.
func (e *editor) viKillLine() {
	if e.pos > e.viInsertStart {
		e.killTo(e.viInsertStart)
	}
}

// viBackwardKillWord is `^W` in vi insert mode: the vi word before the cursor,
// stopping where this stretch of insert mode began.
func (e *editor) viBackwardKillWord() {
	if e.pos > e.viInsertStart {
		e.killTo(max(viBackwardWord(e.line, e.pos, false), e.viInsertStart))
	}
}

// redisplay draws the whole prompt and the line again from the prompt's first
// row, as zsh's `redisplay` does: up to the top, the screen cleared below, and
// everything written fresh.
func (e *editor) redisplay(prompt drawnPrompt) {
	if e.cols() <= 0 {
		e.redraw(prompt)
		return
	}
	var b strings.Builder
	m := e.moves()
	b.WriteString("\r")
	if up := e.row + leadRows(prompt.lead); up > 0 {
		b.WriteString(m.upBy(up))
	}
	b.WriteString(m.eraseToScreenEnd())
	b.WriteString(prompt.lead)
	e.write(b.String())
	e.row = 0
	e.drawn.valid = false
	e.redraw(prompt)
}

// zshViEscape answers an Escape in vi insert mode that arrived with more
// bytes behind it, and reports whether it did. zsh reads it as a key sequence
// only while the bytes so far begin one its viins keymap has — the arrows,
// `^[O` and `^[[`, and the bracketed paste — and otherwise the Escape is the
// mode switch and the byte after it is a command-mode key. Measured
// 2026-10-06 through a pseudo-terminal against zsh 5.9.2 with `bindkey -v`:
// `aa` then ESC, `A` and `b` in one write, then Backspace twice, deletes the
// `b` and rings — `A` began a new stretch of insert mode — where this read
// `ESC A` as one key, did nothing with it, and stayed in insert mode (#6272).
func (e *editor) zshViEscape(prompt drawnPrompt) bool {
	if !e.zshViInsert || !e.viEditing() || e.viCommand {
		return false
	}
	b, got := e.readByte()
	if got != keyContinues {
		return false
	}
	e.pushBack(b)
	if b == '[' || b == 'O' {
		return false
	}
	e.enterViCommand(prompt)
	return true
}

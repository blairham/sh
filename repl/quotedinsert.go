// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

// quotedInsert puts the next key in the line as the character it is: zsh's
// `quoted-insert`, on `^V` in the emacs keymap (#6241).
//
// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2, `zsh -f`,
// `ab` with the cursor after the `a`, the line read back with `${(q+)BUFFER}`:
//
//	keys            line            cursor
//	^V ^A           a ^A b          2     ← drawn as a reverse-video `^A`
//	^V a            aab             2
//	^V ESC          a ^[ b          2
//	^V ^V           a ^V b          2
//	^V Return       a ^M b          2     ← not accepted
//	^V Tab          a TAB b         2     ← not completed
//	^V DEL          a ^? b          2
//	^V é            aéb             2
//	^V ESC [ A      a ^[ [A b       4     ← one byte: the rest is typed
//	ESC 3 ^V ^A     three ^As       4
//	ESC - ^V ^A     one, cursor before it    1
//	ESC 0 ^V ^A     nothing         1
//	^V ^C           a bell, and the line abandoned (bash: a ^C typed)
//
// Nothing is drawn while it waits for the key. The count is the one typing
// takes, which is why it goes through typeCounted.
func (e *editor) quotedInsert(prompt drawnPrompt) {
	if n := e.countAsGiven(); n < 0 && e.negativeTypesNothing {
		// A negative count is that many keys, each put in the line once and
		// the cursor after it. See EditorStyle.NegativeCountTypesNothing.
		for range -n {
			if e.insertQuoted(prompt, false); e.interruptRequested {
				return
			}
		}
		return
	}
	e.insertQuoted(prompt, false)
}

// viQuotedInsert is quotedInsert in vi insert mode: zsh's `vi-quoted-insert`,
// on `^V` in the viins keymap (#6251).
//
// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2, a two-row
// prompt, `bindkey -v`: the one difference is what is drawn while it waits.
// `abc` with the cursor on the `b`, `^V` draws the row as `a^bc` with the
// cursor on the `^` — a plain caret put in the line, not one written over the
// character under the cursor — and the key that follows takes its place:
// `x` gives `axbc`, `^A` a `^A` in standout, ESC a `^[` and no change of mode.
// `^V ^C` rings, takes the caret away and abandons the line. The caret is
// drawn and never typed: it is not in the line the key goes into, and not a
// change to undo. Without a terminal there is nothing to draw it on, and the
// echoed transcript would keep it, so it is left out there.
func (e *editor) viQuotedInsert(prompt drawnPrompt) {
	if e.cols() <= 0 {
		e.insertQuoted(prompt, false)
		return
	}
	line, pos := e.line, e.pos
	e.line = append(append(append(make([]rune, 0, len(line)+1), line[:pos]...), '^'), line[pos:]...)
	e.redraw(prompt)
	e.line, e.pos = line, pos
	e.insertQuoted(prompt, true)
}

// insertQuoted reads the key quotedInsert and viQuotedInsert put in the line
// and puts it there. placeholder is whether a caret was drawn while it waited,
// which a key that puts nothing in the line has to take away again.
func (e *editor) insertQuoted(prompt drawnPrompt, placeholder bool) {
	b, got := e.readByte()
	switch got {
	case keyAbandoned:
		if !e.quotedInsertAbandons {
			// Typed like any other key. See
			// EditorStyle.QuotedInsertAbandonsOnControlC.
			b = ctrlC
			break
		}
		// Measured in both keymaps: `^V ^C` rings where `^C` alone does not.
		e.ring()
		if placeholder {
			e.redraw(prompt)
		}
		e.interruptRequested = true
		return
	case keyContinues:
	default:
		if placeholder {
			e.redraw(prompt)
		}
		return
	}
	r := rune(b)
	if b >= 0x80 {
		var err error
		if r, err = e.readTypedRune(b); err != nil {
			if placeholder {
				e.redraw(prompt)
			}
			return
		}
	}
	n := e.countAsGiven()
	if n == 0 && placeholder {
		e.redraw(prompt)
	}
	if n < 0 && e.negativeTypesNothing {
		// One of the keys a negative count takes. See quotedInsert.
		e.change(e.typedBefore, func() { e.insert(r) })
		e.redraw(prompt)
		return
	}
	e.typeCounted(r, n, prompt)
}

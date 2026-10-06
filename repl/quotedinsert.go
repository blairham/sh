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
//	^V ^C           the line abandoned
//
// Nothing is drawn while it waits for the key. The count is the one typing
// takes, which is why it goes through typeCounted.
func (e *editor) quotedInsert(prompt drawnPrompt) {
	b, got := e.readByte()
	switch got {
	case keyAbandoned:
		e.interruptRequested = true
		return
	case keyContinues:
	default:
		return
	}
	r := rune(b)
	if b >= 0x80 {
		var err error
		if r, err = e.readTypedRune(b); err != nil {
			return
		}
	}
	e.typeCounted(r, e.countAsGiven(), prompt)
}

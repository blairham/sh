// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"unicode/utf8"
)

// A count typed before a key: ESC and a digit, or ESC and a minus, where the
// dialect has them (EditorStyle.PrefixArgument).
//
// Measured 2026-10-02 on zsh 5.9.2 through a pseudo-terminal, emacs keymap,
// the line read back in zle-line-finish (#5498):
//
//	abcdef ESC 3 ^B X        abcXdef       the key is played three times
//	echo ESC 4 z             echo zzzz
//	ESC 1 ESC 2 z            twelve z      digits accumulate
//	ESC 1 2 z                2z            a plain digit is typing, and spends it
//	ESC 0 z                  nothing       a count of nought
//	ESC - ESC 2 ^B X         X             minus two: ^B goes forwards
//	abcdef ^A ESC - ESC 2 ^F ...           ^F goes backwards, and the deletes
//	                                       swap the same way
//	abc ESC - z              abcz, cursor 3   a negative count types the
//	                                       character and leaves the cursor
//	                                       before it
//	ab ESC 3 ^T              ba
//	abc ESC 2 ← X            aXbc          a key sent as a sequence too
//	ESC 5 then a widget      $NUMERIC=5, the widget called once
//	ESC - then a widget      $NUMERIC=-1
//
// So a count is spent by the next keystroke, which is played as many times as
// it says — except a widget of the shell's, which is called once and told the
// count, and a negative count, which turns the moves and deletes that have an
// opposite into it.

// prefixCount is a count being typed.
type prefixCount struct {
	on       bool
	negative bool
	digits   bool
	value    int
}

// add takes one character of the count: a digit, or the minus that makes it
// negative.
func (c *prefixCount) add(b byte) {
	if !c.on {
		*c = prefixCount{on: true}
	}
	if b == '-' {
		c.negative = true
		return
	}
	c.digits = true
	c.value = c.value*10 + int(b-'0')
}

// n is the count: one where only a minus was typed.
func (c prefixCount) n() int {
	v := c.value
	if !c.digits {
		v = 1
	}
	if c.negative {
		return -v
	}
	return v
}

// opposites is the keys a negative count turns into another key: the moves
// and the deletes that have one.
var opposites = map[byte]byte{
	ctrlB: ctrlF, ctrlF: ctrlB,
	backspace: ctrlD, del: ctrlD, ctrlD: backspace,
}

// spendCount is the count meeting the first byte of a keystroke. It reports
// whether the keystroke has been dealt with entirely, and otherwise the byte
// to treat it as.
//
// ESC goes on: it may be more of the count, and a sequence spends the count
// in escape. Everything else spends it here.
func (e *editor) spendCount(b byte, prompt drawnPrompt) (bool, byte) {
	e.keyNumeric = nil
	if !e.count.on || b == esc {
		return false, b
	}
	n := e.count.n()
	e.count = prefixCount{}
	e.keyNumeric = &n
	if b >= 0x20 && b != del {
		r, err := e.readTypedRune(b)
		if err != nil {
			return true, b
		}
		e.typeCounted(r, n, prompt)
		return true, b
	}
	if o, ok := opposites[b]; ok && n < 0 {
		// And it is the opposite key that is played again, so the record of
		// what was read says so.
		b, n = o, -n
		e.keyBytes[0] = b
	}
	if !repeatable[b] {
		// Return, an interrupt, a completion: a key whose action has no
		// "again" is done once, whatever the count.
		return false, b
	}
	if n == 0 {
		return true, b
	}
	if n < 0 {
		n = -n
	}
	e.countRepeat = n - 1
	return false, b
}

// repeatable is the control keys a count plays more than once: the moves,
// the deletes, the kills and the transpose. Everything else is done once.
var repeatable = map[byte]bool{
	ctrlB: true, ctrlF: true, ctrlD: true, backspace: true, del: true,
	ctrlT: true, ctrlW: true, ctrlK: true, ctrlY: true,
}

// spendCountOnEscape is a sequence spending the count: it is played as many
// times as the count says, and a count of nought or less is played once.
//
// It reports whether the count was negative.
func (e *editor) spendCountOnEscape() bool {
	if !e.count.on {
		return false
	}
	n := e.count.n()
	e.count = prefixCount{}
	e.keyNumeric = &n
	negative := n < 0
	if negative {
		n = -n
	}
	if n > 1 {
		e.countRepeat = n - 1
	}
	return negative
}

// spendCountOnBinding is a bound key spending a count that its first byte
// did not — one that begins with ESC, which spendCount leaves for later. A
// widget of the shell's is told the count and called once; one of the
// editor's is played as many times as it says.
func (e *editor) spendCountOnBinding(b Binding) {
	if e.countKey {
		e.countKey = false
		return
	}
	if !e.count.on {
		return
	}
	n := e.count.n()
	e.count = prefixCount{}
	e.keyNumeric = &n
	if b.Function != "" {
		return
	}
	if n < 0 {
		n = -n
	}
	if n > 1 {
		e.countRepeat = n - 1
	}
}

// escapeOpposites is opposites for the keys sent as ESC and one byte: the
// word moves. Measured: `ab cd ef ^A ESC - ESC 2 ESC b X` puts the X two
// words forward.
var escapeOpposites = map[byte]byte{'b': 'f', 'f': 'b', 'B': 'F', 'F': 'B'}

// typeCounted types a character as many times as the count says, and for a
// negative count leaves the cursor in front of what it typed.
func (e *editor) typeCounted(r rune, n int, prompt drawnPrompt) {
	if n == 0 {
		return
	}
	k := n
	if k < 0 {
		k = -k
	}
	at := e.pos
	e.change(false, func() {
		for range k {
			e.insert(r)
		}
	})
	if n < 0 {
		e.pos = at
	}
	e.typing = true
	e.redraw(prompt)
}

// replayCountedKey plays the keystroke that spent a count again, as many more
// times as the count says, by putting its bytes back to be read.
//
// A keystroke that was more of the count is not replayed, and nor is a
// widget of the shell's: zsh calls one of those once and tells it the count.
func (e *editor) replayCountedKey() {
	if e.countRepeat <= 0 {
		return
	}
	k := e.countRepeat
	e.countRepeat = 0
	if e.keyBinding != nil && e.keyBinding.Function != "" {
		return
	}
	if len(e.keyBytes) == 0 || !utf8.Valid(e.keyBytes) && e.keyBytes[0] != esc {
		return
	}
	e.pushKeys(strings.Repeat(string(e.keyBytes), k))
}

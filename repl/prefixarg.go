// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"fmt"
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

// addAsReadline is add under EditorStyle.CountReadAsReadline, where meta says
// whether the character came with an ESC. A minus after a digit is not more
// of the count and never reaches here: see minusIsTyped.
func (c *prefixCount) addAsReadline(b byte, meta bool) {
	if !c.on {
		*c = prefixCount{on: true}
	}
	if b == '-' {
		if meta && c.on && c.negative {
			// `ESC - ESC -` is one again.
			c.negative = false
			return
		}
		if !c.digits {
			c.negative = true
		}
		return
	}
	d := int(b - '0')
	switch {
	case c.digits:
		c.value = c.value*10 + d
	case meta && c.negative:
		// An ESC digit after a lone minus is appended to the minus's one:
		// `ESC - ESC 2` is -12.
		c.value = 10 + d
	default:
		c.value = d
	}
	c.digits = true
}

// minusIsTyped reports whether a minus is a character to type with the count
// rather than more of it: under readline's rules, once a digit has been
// typed. See EditorStyle.CountReadAsReadline.
func (e *editor) minusIsTyped(b byte) bool {
	return e.countAsReadline && b == '-' && e.count.on && e.count.digits
}

// countMore adds a keystroke's digit or minus to the count.
//
// The keystroke is not a widget call a kill can see: measured 2026-10-04
// against zsh 5.9.2, `aa bb cc`, `^W`, `ESC 2 ^W` and a yank bring back
// `aa bb cc` as one kill (#5918). So whatever run of kills was going on
// before it is still going on after it.
//
// The same for a walk of insert-last-word: `M-. M-2 M-.` is the second word
// from the end of the line before the one the first press used, measured
// 2026-10-05 against zsh 5.9.2 — `two` with `: one two three` before `: alpha
// beta gamma` — and not a second copy (#5987).
func (e *editor) countMore(b byte) {
	if e.countAsReadline {
		e.count.addAsReadline(b, true)
	} else {
		e.count.add(b)
	}
	e.countShown()
	e.killing = e.killedBefore
	if e.lastWordArguments || e.yankCount {
		e.lastArg.walking = e.lastArg.walkingBefore
	}
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
	if e.countAsReadline && (b >= '0' && b <= '9' || b == '-' && !e.count.digits) {
		// Once a count has begun, a plain digit is more of it, and a minus
		// before any digit is absorbed. See EditorStyle.CountReadAsReadline.
		e.count.addAsReadline(b, false)
		e.countShown()
		return true, b
	}
	e.countSpent()
	if e.rebound(b) {
		// A key somebody rebound is what they bound it to, count and all:
		// the count is left for the binding to spend (see
		// spendCountOnBinding), and nothing here turns the key into
		// another by its byte. Measured 2026-10-05 against zsh 5.9.2, a
		// widget of the shell's on `^B` is told `NUMERIC=-1` for `ESC -
		// ^B`, where this played `^F` instead (#6031).
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
	if b == ctrlK && e.killLineSign {
		// Back to the start of the line for a negative count, as `^U` does,
		// and to the end once for any other. See
		// EditorStyle.KillLineReadsOnlyTheSign.
		w := WidgetKillLine
		if n < 0 {
			w = WidgetKillWholeLine
		}
		e.runWidget(Binding{Widget: w}, prompt)
		return true, b
	}
	if b == ctrlT && n <= 0 && e.transposeNoNegative {
		// Nothing, except at the end of the line. See
		// EditorStyle.TransposeCharsTakesNoNegativeCount.
		if e.pos == len(e.line) {
			e.runWidget(Binding{Widget: WidgetTransposeChars}, prompt)
		}
		return true, b
	}
	if o, ok := opposites[b]; ok && n < 0 {
		if e.rebound(o) {
			// The opposite key means something else now, so the opposite
			// *action* is performed rather than the key: measured, `abc
			// ESC - Backspace` with `^D` rebound deletes forward and leaves
			// the binding on `^D` alone.
			for range -n {
				e.runWidget(Binding{Widget: oppositeActions[b]}, prompt)
			}
			return true, b
		}
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

// oppositeActions is the action each key in opposites turns into, for when
// the opposite key has been rebound.
var oppositeActions = map[byte]Widget{
	ctrlB: WidgetForwardChar, ctrlF: WidgetBackwardChar,
	backspace: WidgetDeleteChar, del: WidgetDeleteChar, ctrlD: WidgetBackwardDeleteChar,
}

// rebound reports whether a key's first byte begins something in the
// override table, which is what decides that the byte is not the editor's.
func (e *editor) rebound(b byte) bool {
	if e.bindings == nil {
		return false
	}
	table := e.bindings(e.keymap())
	return len(table) > 0 && anyBindingStartsWith(table, string(b))
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
	e.countSpent()
	n := e.count.n()
	e.count = prefixCount{}
	// The count as it was typed, sign and all, and not the variable the
	// replay below turns positive: insert-last-word reads it, and `M-- M-1
	// M-.` is the first word and not the last (#5987).
	told := n
	e.keyNumeric = &told
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
	e.countSpent()
	n := e.count.n()
	e.count = prefixCount{}
	// The count as it was typed, sign and all, and not the variable the
	// replay below turns positive: an action that reads the count itself
	// is told -1 for `ESC -` (#5960).
	told := n
	e.keyNumeric = &told
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
	if n == 0 || n < 0 && e.negativeTypesNothing {
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
// widget of the shell's, or an action that reads the count itself: zsh calls
// one of those once and tells it the count.
func (e *editor) replayCountedKey() {
	if e.countRepeat <= 0 {
		// The run, if there was one, is over.
		e.countRunning, e.countActed = false, false
		return
	}
	k := e.countRepeat
	e.countRepeat = 0
	if e.keyBinding != nil && (e.keyBinding.Function != "" || e.keyBinding.Widget.takesItsCount()) {
		return
	}
	if len(e.keyBytes) == 0 || !utf8.Valid(e.keyBytes) && e.keyBytes[0] != esc {
		return
	}
	if e.countStops {
		// One press at a time, so that a press with nothing to act on can
		// stop the rest — see ringUnless. And a counted `^T` stops at the end
		// of the line. See EditorStyle.CountStopsWhereItCannotAct.
		if string(e.keyBytes) == string([]byte{ctrlT}) && e.pos == len(e.line) {
			e.countRunning, e.countActed = false, false
			return
		}
		e.countRepeat = k - 1
		e.pushKeys(string(e.keyBytes))
		return
	}
	e.pushKeys(strings.Repeat(string(e.keyBytes), k))
}

// countShown draws the count where the dialect draws one, by putting a draw
// in hand: see EditorStyle.CountPrompt, and promptForCount.
func (e *editor) countShown() {
	if e.countPrompt != "" {
		e.pendingDraw = true
	}
}

// countSpent is the count about to be spent by the keystroke in hand. Where
// it was drawn, the prompt goes back whatever the keystroke draws; and where a
// counted key stops at the edge, the run begins (see ringUnless).
func (e *editor) countSpent() {
	if e.countPrompt != "" {
		e.pendingDraw = true
	}
	if e.countStops {
		e.countRunning, e.countActed = true, false
	}
}

// promptForCount is the prompt with its last row given to the count being
// typed, where the dialect draws one. See EditorStyle.CountPrompt.
func (e *editor) promptForCount(prompt drawnPrompt) drawnPrompt {
	if e.countPrompt == "" || !e.count.on {
		return prompt
	}
	text := fmt.Sprintf(e.countPrompt, e.count.n())
	prompt.text, prompt.cells = text, displayWidth(text)
	return prompt
}

// countRunsOut is a press of a counted key that had nothing to act on, under
// EditorStyle.CountStopsWhereItCannotAct: the rest of the run is dropped, and
// it reports whether the bell is owed — when no press acted at all, or when
// the key goes backward a character at a time.
func (e *editor) countRunsOut() bool {
	ring := !e.countActed || ringsWhenACountRunsOut[string(e.keyBytes)]
	e.countRepeat, e.countRunning, e.countActed = 0, false, false
	return ring
}

// ringsWhenACountRunsOut are the keys whose counted run rings when it reaches
// the start of the line part-way: `^B`, Left, Backspace and `^H`. The rest
// ring only if they could not act at all.
var ringsWhenACountRunsOut = map[string]bool{
	string([]byte{ctrlB}): true, "\x1b[D": true, "\x1bOD": true,
	string([]byte{del}): true, string([]byte{backspace}): true,
}

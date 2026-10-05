// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strconv"
	"unicode"
)

// `M-.` — the last argument of the line before, put in at the cursor.
//
// `cd some/deep/path` and then `ls M-.`, which is the shape of half the
// commands anyone types: the argument is long, it was just typed, and typing
// it again is where the mistake comes from. `M-_` is the same key; both
// spellings are bound in both shells.
//
// Pressed again straight away it walks further back, replacing what it put in
// rather than adding a second copy — so the presses count lines and not words.
// A keystroke in between ends the walk, and measured, the press after that one
// starts again at the most recent line and inserts a second copy. The walk is
// therefore a property of the *previous keystroke* and not of the line, which
// is what lastArgWalk keeps.

// lastArgWalk is where `M-.` has got to.
type lastArgWalk struct {
	// walking says this keystroke was `M-.`; walkingBefore says the one
	// before it was, which is what makes this press walk rather than insert.
	// The pair is set the way killing and killedBefore are, at the top of the
	// read loop, because the state a key needs is what the key before it left.
	walking, walkingBefore bool

	// back is how many lines back the walk has reached, counting the previous
	// line as 1. at and length are where in the line the text it inserted
	// sits, so the next press can take that text out again.
	back, at, length int
}

// insertLastArg is `M-.`.
func (e *editor) insertLastArg(prompt drawnPrompt) {
	if e.lastWordArguments {
		e.actionStatus = e.insertLastWordWith(e.actionArgs, e.keyNumeric, prompt)
		return
	}
	back := 1
	if e.lastArg.walkingBefore {
		back = e.lastArg.back + 1
	}
	word := ""
	switch {
	case back <= len(e.history):
		word = lastWord(e.history[len(e.history)-back])
	case e.lastArg.walkingBefore && e.lastArgStaysOnOldest && len(e.history) > 0:
		// zsh: the walk stops at the oldest line and stays on it, so a press
		// too many leaves the line exactly as the press before it did.
		back = len(e.history)
		word = lastWord(e.history[0])
	case !e.lastArg.walkingBefore:
		// Nothing behind the prompt at all. Both shells do nothing, and the
		// walk does not start, so a second press does nothing either.
		return
	}
	// bash falls through to here with no word: past the oldest line it takes
	// what it had inserted back off and puts nothing in its place, and every
	// further press leaves it that way. The counter keeps rising rather than
	// being clamped, which is what stops the next press coming back around to
	// the most recent line.

	e.change(e.lastArg.walkingBefore, func() {
		at := e.pos
		if e.lastArg.walkingBefore {
			at = e.lastArg.at
			e.line = append(e.line[:at], e.line[at+e.lastArg.length:]...)
			e.pos = at
		}
		for _, r := range word {
			e.insert(r)
		}
		e.lastArg.back, e.lastArg.at, e.lastArg.length = back, at, len([]rune(word))
	})
	e.lastArg.walking = true
	e.redraw(prompt)
}

// insertLastWordWith is zsh's insert-last-word, given the count and the
// arguments a widget's call named, and it answers the call's status.
//
// The walk position is how many lines back the last call looked, 0 being the
// line being edited, and a call that does not follow another starts from 0 —
// whichever entry the history walk is on, measured: Up and then `M-.` inserts
// the newest line's last word. The arguments are zshzle(1)'s: the offset the
// walk moves by (-1 back, 1 forward, 0 the same line again), the word in
// array notation (-1 the last, 1 the first), and a third of any value saying
// the offset counts from the line being edited instead. Without a word, the
// count picks it: a positive count from the end of the line and nought or a
// negative one from the start, nought being the command word.
//
// Measured 2026-10-05 through a pseudo-terminal against zsh 5.9.2, emacs
// keymap, with `: one two three` and then `: alpha beta gamma` in the
// history:
//
//	M-. M-.                         three
//	M-2 M-. / M-. M-2 M-.           beta / two
//	M-0 M-. / M-- M-. / M-- M-1 M-. :  / alpha / alpha
//	M-5 M-. / `-- -1 5`             nothing, status 1
//	`-- -1 -2`, twice               beta, then two in its place
//	M-. then `-- 0 -3`              alpha: the same line again
//	`-- 0 -1` with no call before   nothing on an empty line: line 0 is the
//	                                line being edited, and has no words
//	M-. then `-- 1 -1`              the word taken back off and nothing put
//	`-- 1 -1` with no call before   nothing, status 1: no line ahead of 0
//	M-. M-. then `-- -1 -1 1`       gamma in place of three
//	M-. M-. M-. M-.                 three: past the oldest line nothing
//	                                changes; then `-- 1 -1` is gamma
//	M-. then `-- -1 5` then `-- 0 -3`  one: a line without the word leaves
//	                                the line alone and the walk on it
//	M-. then `-- -1 5` then M-.     gamma: from there, past the oldest
//	`-- x y`                        nothing, status 1
//	M-2 then `-- -1`                beta: with no word named, the count
//	`zle insert-last-word` twice    three, from one widget call
//	`-- -1 0`                       gamma: word 0 is the last
//
// Not modeled: zsh's answer to the third argument while the history is being
// walked, which none of the rows above settled.
func (e *editor) insertLastWordWith(args []string, numeric *int, prompt drawnPrompt) int {
	// The count is spent here, whatever it picked: a key pressed with one
	// is performed once and not played again.
	e.countRepeat = 0
	offset, word, fromCurrent := -1, -1, false
	if numeric != nil {
		if n := *numeric; n > 0 {
			word = -n
		} else {
			word = 1 - n
		}
	}
	if len(args) > 0 {
		var ok bool
		if offset, ok = atoiStrict(args[0]); !ok {
			return 1
		}
	}
	if len(args) > 1 {
		var ok bool
		if word, ok = atoiStrict(args[1]); !ok {
			return 1
		}
		if word == 0 {
			word = -1
		}
	}
	fromCurrent = len(args) > 2
	walking := e.lastArg.walkingBefore
	start := 0
	if walking && !fromCurrent {
		start = e.lastArg.back
	}
	// Still a call of this action, which is what the next one asks, whether
	// or not this one finds a word.
	e.lastArg.walking = true
	back := start - offset
	if back < 0 || back > len(e.history) {
		// No such line, ahead of the one being edited or behind the oldest:
		// nothing changes, the walk included, so `M-.` pressed past the
		// oldest line leaves its word there and the walk where it was.
		return 1
	}
	// Line 0 is the line being edited, as it stands without what the last
	// call put in it: `p q r ` and then `-- 0 -1` inserts `r`, and `-- 0 -2`
	// after that puts `q` in its place. A word it has not got is nothing in
	// place of what was there, where a history line that has not got one
	// leaves the line alone — `M-.` then `-- 1 -1` takes `gamma` back off.
	at, current := e.pos, e.line
	if walking {
		at = e.lastArg.at
		current = append(append([]rune(nil), e.line[:at]...), e.line[at+e.lastArg.length:]...)
	}
	source := string(current)
	if back > 0 {
		source = e.history[len(e.history)-back]
	}
	words := lineWords(source)
	i := word - 1
	if word < 0 {
		i = len(words) + word
	}
	text := ""
	switch {
	case i >= 0 && i < len(words):
		text = words[i]
	case back > 0:
		// The line has no such word. Nothing changes but the walk, which
		// is on the line it looked at.
		e.lastArg.back = back
		return 1
	}
	e.change(walking, func() {
		e.line, e.pos = current, at
		for _, r := range text {
			e.insert(r)
		}
		e.lastArg.back, e.lastArg.at, e.lastArg.length = back, at, len([]rune(text))
	})
	e.redraw(prompt)
	return 0
}

// atoiStrict reads a whole word as a decimal number, sign and all.
func atoiStrict(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	return n, err == nil
}

// histNo is the number of the history line being edited — see Line.HistNo.
func (e *editor) histNo() int {
	if e.historyCount == nil {
		return 0
	}
	if n := e.historyCount() + 1 - (len(e.history) - e.browsing); n > 0 {
		return n
	}
	return 0
}

// lastWord is the last argument of a line, as `M-.` inserts it.
//
// Whitespace separates the words, except inside quotes, and the quotes come
// along with the word. Measured in both shells: `: p 'x y'` gives back
// `'x y'` — the apostrophes and the space between them — and `: p a\ b` gives
// back `b`, so a quote holds a word together and a backslash does not.
//
// A line with one word on it gives that word back, which is the command name:
// measured, `M-.` after `: solo` inserts `solo`.
func lastWord(line string) string {
	words := lineWords(line)
	if len(words) == 0 {
		return ""
	}
	return words[len(words)-1]
}

// lineWords is a line's words, split the way lastWord describes.
func lineWords(line string) []string {
	rs := []rune(line)
	var words []string
	for i := 0; i < len(rs); {
		for i < len(rs) && unicode.IsSpace(rs[i]) {
			i++
		}
		if i == len(rs) {
			break
		}
		start := i
		var quote rune
		for i < len(rs) && (quote != 0 || !unicode.IsSpace(rs[i])) {
			switch {
			case quote == rs[i]:
				// The closing half of the pair that opened this one.
				quote = 0
			case quote == 0 && (rs[i] == '\'' || rs[i] == '"'):
				quote = rs[i]
			}
			i++
		}
		// A quote nobody closed runs to the end of the line, which is the only
		// answer available and also what a shell would make of the line.
		words = append(words, string(rs[start:i]))
	}
	return words
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "unicode"

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
	rs := []rune(line)
	last := ""
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
		last = string(rs[start:i])
	}
	return last
}

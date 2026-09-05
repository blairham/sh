// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"unicode"
)

// Where a word begins and ends, and what killing one leaves behind.
//
// This is the half of line editing that a person uses without thinking about
// it: `M-b` back over the argument that was mistyped, `^W` to take the last
// path off the line, `^Y` to put it back somewhere else. All of it turns on
// one question — which characters belong to a word — and the two dialects
// answer it differently, so the answer is a field rather than a constant.

// isWordRune reports whether the character belongs to a word.
//
// Letters and digits always do. Beyond that it is the dialect's list: bash
// counts nothing else, so `M-b` on `/usr/local/bin` stops at `bin`; zsh counts
// most punctuation, so the same keystroke goes to the start of the path.
func (e *editor) isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(e.wordChars, r)
}

// backwardWord is where `M-b` goes: back over whatever is not a word, then
// back over the word itself.
func (e *editor) backwardWord() int {
	i := e.pos
	for i > 0 && !e.isWordRune(e.line[i-1]) {
		i--
	}
	for i > 0 && e.isWordRune(e.line[i-1]) {
		i--
	}
	return i
}

// endOfWord is the far end of the word the cursor is in or the next one after
// it. It is where `M-d` kills to in both dialects, and where `M-f` goes in one
// of them.
func (e *editor) endOfWord() int {
	i := e.pos
	for i < len(e.line) && !e.isWordRune(e.line[i]) {
		i++
	}
	for i < len(e.line) && e.isWordRune(e.line[i]) {
		i++
	}
	return i
}

// forwardWord is where `M-f` goes, and the two dialects part company here.
//
// Measured on `echo one two` with the cursor at the start: bash leaves it
// after `echo` and zsh leaves it before `one`. Same key, same line, one
// character apart every time, which is exactly the kind of difference that
// makes a shell feel like somebody else's.
func (e *editor) forwardWord() int {
	if !e.forwardWordStopsBeforeNext {
		return e.endOfWord()
	}
	i := e.pos
	for i < len(e.line) && e.isWordRune(e.line[i]) {
		i++
	}
	for i < len(e.line) && !e.isWordRune(e.line[i]) {
		i++
	}
	return i
}

// wordStartBeforeCursor is where `^W` kills back to, and this is the second
// place the dialects disagree.
//
// Measured on `echo a+b`: bash leaves `echo ` and zsh leaves `echo a+`. bash's
// is delimited by whitespace and nothing else, which is what makes it the key
// that takes a whole path off the line whatever punctuation is in it; zsh's is
// the same word its motion keys use.
func (e *editor) wordStartBeforeCursor() int {
	if e.killBeforeCursorUsesWords {
		return e.backwardWord()
	}
	i := e.pos
	for i > 0 && unicode.IsSpace(e.line[i-1]) {
		i--
	}
	for i > 0 && !unicode.IsSpace(e.line[i-1]) {
		i--
	}
	return i
}

// kill remembers text that has just been removed, so that `^Y` can put it
// back.
//
// One buffer rather than a ring with `M-y` to walk it: the ring is a second
// surface with its own key, and what a person reaches for after a mistaken
// `^W` is the thing they just lost. Two kills in a row join into one piece —
// `^W^W^Y` brings both words back in the order they were on the line, which is
// why the direction matters — and anything else between them starts afresh.
//
// Measured, and the measurement needed care: typed as a burst, bash appears to
// join kills across an insert between them. Typed a keystroke at a time, which
// is how a person types, both shells start afresh. The burst is read out of
// pending input by a path that never sees the keystroke in between, so the
// first reading was of the harness rather than of the shell.
func (e *editor) kill(text []rune, forward bool) {
	wasKilling := e.killedBefore
	e.killing = true
	switch {
	case !wasKilling:
		e.killed = append([]rune(nil), text...)
	case forward:
		e.killed = append(e.killed, text...)
	default:
		// A backward kill goes in front of what is already there, so that two
		// of them read in the order the words were written.
		e.killed = append(append([]rune(nil), text...), e.killed...)
	}
}

// killTo removes what lies between i and the cursor, and leaves the cursor at
// i.
//
// With nothing between them it is not a kill at all: what `^Y` holds is left
// alone — measured, `^K` at the end of a line does not empty it — and the kill
// after it starts afresh rather than joining, so `^W`, `^K`, `^W` is two
// pieces rather than one.
//
// That second half is the one disagreement about editing that is not a field.
// bash starts afresh and zsh joins; a field would have to be answered by every
// dialect ever added, for a sequence of keystrokes nobody types. bash's
// answer, written down here and in docs/spec/editing.md rather than taken
// quietly.
func (e *editor) killTo(i int) {
	if i < 0 || i >= e.pos {
		return
	}
	e.kill(e.line[i:e.pos], false)
	e.line = append(e.line[:i], e.line[e.pos:]...)
	e.pos = i
}

// killForwardTo removes what lies between the cursor and j.
func (e *editor) killForwardTo(j int) {
	if j > len(e.line) || j <= e.pos {
		return
	}
	e.kill(e.line[e.pos:j], true)
	e.line = append(e.line[:e.pos], e.line[j:]...)
}

// killToStart is `^U`, and it is the loudest of the disagreements.
//
// Measured with the cursor at the start of `echo one two`: bash leaves the
// line alone, because there is nothing before the cursor to kill; zsh empties
// it. A finger that means one and gets the other loses a whole command.
func (e *editor) killToStart() {
	if e.wholeLineKill {
		// The whole line as one backward kill from its end, so that an empty
		// line is a kill of nothing here as much as it is anywhere else.
		e.pos = len(e.line)
	}
	e.killTo(0)
}

// yank puts the last kill back at the cursor.
func (e *editor) yank() {
	n := len(e.killed)
	if n == 0 {
		return
	}
	e.line = append(e.line, make([]rune, n)...)
	copy(e.line[e.pos+n:], e.line[e.pos:])
	copy(e.line[e.pos:], e.killed)
	e.pos += n
}

// transpose swaps the two characters around the cursor and steps past them —
// `^T`, which is what fixes a `pattren` without retyping the word.
//
// At the very start of the line there is nothing before the cursor to swap,
// and the dialects answer that differently: measured on `echo abc` with the
// cursor at the start, bash leaves the line alone and zsh swaps the first two
// characters and moves the cursor past them.
func (e *editor) transpose() {
	if len(e.line) < 2 {
		return
	}
	switch {
	case e.pos == 0:
		if !e.transposeAtStart {
			return
		}
		e.line[0], e.line[1] = e.line[1], e.line[0]
		e.pos = 2
	case e.pos >= len(e.line):
		// At the end there is nothing under the cursor, so the two before it
		// are the ones that swap and the cursor stays where it is.
		n := len(e.line)
		e.line[n-2], e.line[n-1] = e.line[n-1], e.line[n-2]
	default:
		e.line[e.pos-1], e.line[e.pos] = e.line[e.pos], e.line[e.pos-1]
		e.pos++
	}
}

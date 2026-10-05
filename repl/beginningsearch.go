// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "strings"

// Two pairs of actions that a widget function reaches for more than a key
// does: the history walk to entries that begin with what is before the
// cursor, and the move a line up or down inside a line that holds several.
// They are what `up-line-or-beginning-search` and its partner are built from,
// and those are what a great many startup files bind the arrows to (#5910).
//
// Measured 2026-10-04 through a pseudo-terminal against zsh 5.9.2, a widget
// setting the line, calling the action by name and recording the status, one
// keypress per row, with `: echo apple`, `: ls one`, `: echo banana`, `: echo
// apple`, `: ls two`, `: echo cherry` typed into history:
//
//	line @cursor               action                       line, cursor, status
//	: ec @4                    beginning-search-backward    : echo cherry, 4, 0
//	again                                                   : echo apple (the 4th), 4, 0
//	abc @1                     beginning-search-forward     abc, 1, 1 — nothing newer
//	abcdef⏎xy⏎longer line @8   down-line                    11, 0
//	same @12                   up-line                      9 — the column, clamped
//	                                                        to the shorter line
//	same @5                    up-line                      0, 1 — the first line
//	abc @1                     up-line                      0, 1
//	abc @1                     down-line                    1, 1 — the last line
//
// **What is searched for is the text before the cursor**, which is the whole
// of the difference from the matching pair (WidgetPreviousHistoryMatching),
// which looks for the line's first word and ignores the cursor. And the
// cursor stays where it was rather than going to the end — the reason a
// function wrapping this moves it there itself, when it wants it there.
//
// **An entry the same as the line is passed over.** With `: x a`, `: x b`,
// `: x a`, `: x a` in history and `: x` typed, three presses visit the fourth,
// the second and the first: the third is the line the first press left, and
// a search that stopped on it would look like a key that did nothing.
//
// **The bottom of the walk is a candidate like any entry**: walking forward
// past the newest match brings back what was being typed, which is how
// `down-line-or-beginning-search` gets a person back to their own line.
//
// **Up from the first line puts the cursor at the start**, at status 1, where
// down from the last line leaves it alone. The two are not symmetric in zsh
// and are reproduced as measured. What is not reproduced is the column zsh
// keeps across a run of these: a second `down-line` straight after a first
// aims for the column the run started in rather than the one the cursor is
// in. Each call here aims for the column it starts from.

// beginningSearch walks to the nearest entry in direction dir that begins with
// the text before the cursor, and leaves the cursor where it was. The status
// is 1 when nothing does, with the line untouched.
func (e *editor) beginningSearch(dir int, prompt drawnPrompt) int {
	if len(e.history) == 0 {
		return 1
	}
	pos := e.pos
	prefix := string(e.line[:pos])
	current := string(e.line)
	for to := e.browsing + dir; to >= 0 && to <= len(e.history); to += dir {
		candidate, ok := e.searchCandidate(to)
		if !ok || candidate == current || !strings.HasPrefix(candidate, prefix) {
			continue
		}
		// Through browse's own step, so the drafts and what the bottom of
		// the walk brings back are the walk's — then the cursor back where
		// it was, which is the one thing browse does differently.
		e.browse(to-e.browsing, prompt)
		e.moveTo(min(pos, len(e.line)), prompt)
		return 0
	}
	return 1
}

// searchCandidate is the text the walk would bring back at this step: the
// entry, or at the bottom the line that was being typed when the walk began.
func (e *editor) searchCandidate(to int) (string, bool) {
	if to == len(e.history) {
		draft, kept := e.drafts[to]
		return string(draft), kept
	}
	return e.history[to], true
}

// lineMotion moves the cursor one line up or down within the line being
// edited, keeping its column where the line it lands on is long enough.
func (e *editor) lineMotion(dir int, prompt drawnPrompt) int {
	start := rowStart(e.line, e.pos)
	column := e.pos - start
	if dir < 0 {
		if start == 0 {
			e.moveTo(0, prompt)
			return 1
		}
		above := rowStart(e.line, start-1)
		e.moveTo(above+min(column, start-1-above), prompt)
		return 0
	}
	end := rowEnd(e.line, e.pos)
	if end == len(e.line) {
		return 1
	}
	below := end + 1
	e.moveTo(below+min(column, rowEnd(e.line, below)-below), prompt)
	return 0
}

// rowStart is the index the line holding pos begins at.
func rowStart(line []rune, pos int) int {
	for pos > 0 && line[pos-1] != '\n' {
		pos--
	}
	return pos
}

// rowEnd is the index of the newline that ends the line holding pos, or the
// length of the whole line where it is the last.
func rowEnd(line []rune, pos int) int {
	for pos < len(line) && line[pos] != '\n' {
		pos++
	}
	return pos
}

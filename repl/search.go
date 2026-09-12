// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Reverse incremental search: `C-r`.
//
// The single most-used thing anyone does with a shell history, and the reason
// the file is worth keeping at all. The up arrow walks a list; this asks it a
// question, and the difference matters once the list is longer than a screen.
//
// A mode rather than a key, which is why it has a loop of its own: while it is
// running, an ordinary character extends the query instead of going into the
// line, and every other key means "stop, and do what you normally do". That
// last part is measured and is the part a naive implementation gets wrong —
// `C-r cho C-e` in both bash and zsh leaves search, keeps the line it found,
// *and* moves the cursor to the end. The key is not swallowed by the mode it
// closed. See pushBack.
//
// In its own file because the key it hangs off is one line of editor.go's
// switch, and the mode is a hundred lines that has nothing else to do with
// how a line is drawn.

// reverseSearch runs the search until something ends it.
//
// The line and the cursor are left where the search wants them, and the byte
// that ended the mode — if it means something outside it — is pushed back for
// the caller's own switch to read on its next turn.
func (e *editor) reverseSearch(prompt drawnPrompt) {
	// What to put back if the search is abandoned. Both shells restore the
	// line as it was before `C-r`, which is the whole use of `C-g`: a search
	// that cannot be backed out of is one people stop starting.
	saved, savedPos, savedAt := append([]rune(nil), e.line...), e.pos, e.browsing
	// Kept where the arrows can find it, so that a search and the up arrow are
	// one walk rather than two: after `C-r` lands on an entry, Down comes back
	// towards the line that was being typed instead of towards nothing.
	e.drafts[e.browsing] = saved

	var query []rune
	// at is the entry the line is showing, and -1 is the caller's own line —
	// which is what an empty query shows, measured: `C-r` on a half-typed
	// line draws the search prompt with that line still after it.
	at, failed := -1, false

	e.drawSearch(prompt, query, failed)
	var buf [1]byte
	for {
		// Through nextByte and not the reader, for the reason confirmList
		// gives: the rest of the search string may already be buffered, and a
		// read of the descriptor would wait for bytes this editor has been
		// given. `C-r` and what follows it arrive in one write from anything
		// but a human.
		n, err := e.nextByte(buf[:])
		if err != nil {
			// The input ended mid-search. The line goes back to what it was,
			// and the caller's own read reports the same error a moment later.
			e.line, e.pos, e.browsing = saved, savedPos, savedAt
			e.redraw(prompt)
			return
		}
		if n == 0 {
			continue
		}
		switch c := buf[0]; c {
		case ctrlR:
			// One more step back. From the entry before the one on the screen,
			// so a repeated `C-r` walks rather than finding the same line
			// again; from the newest when nothing has been found yet.
			from := len(e.history) - 1
			if at >= 0 {
				from = at - 1
			}
			if i, off := e.findBack(query, from); i >= 0 {
				at, failed = i, false
				e.showMatch(i, off)
			} else {
				// The line that did match stays on the screen and the query
				// stays as it was: measured, both shells only change the
				// wording and ring the bell, so the next backspace goes back
				// to a search that works.
				failed = true
				e.write(bell)
			}
		case ctrlG:
			e.line, e.pos, e.browsing = saved, savedPos, savedAt
			e.redraw(prompt)
			return
		case backspace, del:
			if len(query) == 0 {
				// Nothing left to take away. A backspace here is not a
				// request to delete from the line — the line is a search
				// result, not something being typed.
				e.write(bell)
				continue
			}
			query = query[:len(query)-1]
			at, failed = e.research(query, at)
		default:
			if c < 0x20 {
				// Every other control byte means the mode is over and the key
				// meant what it always means. Redrawn with the real prompt
				// first, so the search wording is off the screen before the
				// caller draws anything of its own.
				e.redraw(prompt)
				e.pushBack(c)
				return
			}
			r, err := e.readRune(c)
			if err != nil {
				e.line, e.pos, e.browsing = saved, savedPos, savedAt
				e.redraw(prompt)
				return
			}
			query = append(query, r)
			at, failed = e.research(query, at)
		}
		e.drawSearch(prompt, query, failed)
	}
}

// research answers a changed query, and says where the line came from.
//
// From the entry already on the screen rather than from the newest: extending
// a query that still matches must not jump forward to a more recent line, and
// measured it does not — `C-r echo C-r` lands on the older `echo`, and typing
// another character there stays on it rather than going back to the newer one.
func (e *editor) research(query []rune, at int) (int, bool) {
	from := len(e.history) - 1
	if at >= 0 {
		from = at
	}
	i, off := e.findBack(query, from)
	if i < 0 {
		e.write(bell)
		return at, true
	}
	e.showMatch(i, off)
	return i, false
}

// findBack is the newest entry at or before from that contains the query, and
// where in it the match begins.
//
// Rune offsets out, byte offsets in: the cursor sits between characters, and a
// match after a multi-byte one would otherwise be drawn several columns to the
// right of where it is.
// from is an index inside the list or one below the oldest: both callers start
// from either the entry on the screen or the newest, so there is no clamp here
// and no branch that nothing can reach.
func (e *editor) findBack(query []rune, from int) (int, int) {
	q := string(query)
	for i := from; i >= 0; i-- {
		if j := strings.Index(e.history[i], q); j >= 0 {
			return i, utf8.RuneCountInString(e.history[i][:j])
		}
	}
	return -1, 0
}

// showMatch puts an entry in the line with the cursor at the match.
//
// At the match rather than at either end, which is measured and is the useful
// half: the thing that was searched for is where the edit is about to happen,
// and leaving the cursor there means a search is also a way of getting to a
// place in a long command.
func (e *editor) showMatch(i, off int) {
	e.line = []rune(e.history[i])
	e.pos = min(off, len(e.line))
	// The arrows carry on from here rather than from wherever they were when
	// the search started, which is what both shells do and is the only reading
	// that makes sense: the entry on the screen is the one Up goes back from.
	e.browsing = i
}

// drawSearch puts the search on the screen, in whichever of the two shapes
// this dialect uses.
func (e *editor) drawSearch(prompt drawnPrompt, query []rune, failed bool) {
	wording := or(e.searchPrompt, defaultSearchPrompt)
	if failed {
		wording = or(e.searchFailed, defaultSearchFailed)
	}
	// Through drawPrompt, because the editor's arithmetic wants the cells as
	// well as the bytes and a dialect could put a color in its wording.
	text := drawPrompt(fmt.Sprintf(wording, string(query)))
	if !e.searchBelow || e.cols() <= 0 {
		// The prompt is replaced by the search, which is bash's shape and the
		// substrate's own. It is also the only shape available without a
		// terminal width: a second row cannot be placed under a line whose
		// last row is not known.
		e.redraw(text)
		return
	}
	// zsh's shape: the prompt and the line stay, and the search goes on a row
	// below them. Nothing has to erase that row — the next redraw clears to
	// the end of the screen from the prompt's row, which is where this one
	// starts from too — and that is also what takes it away when the search
	// ends.
	e.redraw(prompt)
	e.below(prompt, text.text)
}

// below draws one row of text under the last row of the line and comes back to
// where the cursor was.
func (e *editor) below(prompt drawnPrompt, text string) {
	cols := e.cols()
	end := (prompt.cells + len(e.line)) / cols
	down := end - e.row + 1
	col := (prompt.cells + e.pos) % cols

	var b strings.Builder
	for range down {
		// A newline rather than a cursor-down, so the row exists: moving down
		// past the bottom of the screen does nothing, and the text would land
		// on the line instead of under it.
		b.WriteString("\r\n")
	}
	b.WriteString("\x1b[K")
	b.WriteString(text)
	b.WriteString("\r\x1b[")
	b.WriteString(itoa(down))
	b.WriteString("A")
	if col > 0 {
		b.WriteString("\x1b[")
		b.WriteString(itoa(col))
		b.WriteString("C")
	}
	e.write(b.String())
}

// pushBack keeps a byte for the caller's next read.
//
// One byte is all this mode ever needs, and it is for the reason the mode
// exists: exactly one key ends the search, and it is that key the caller has
// to see. It goes on the front of the same queue pushKeys uses rather than in
// a field of its own, because two places putting input back with two answers
// to "which comes first" is how a pushed key gets read out of order.
func (e *editor) pushBack(c byte) { e.pushed = append([]byte{c}, e.pushed...) }

// pushKeys is the same for an action outside the editor putting characters
// back — see Actions.PushKeys, and editoractions.go for what reaches it.
//
// In front of whatever is already pushed, and that is measured rather than
// convenient: two pushes inside one widget come back newest first, each push's
// own characters in the order they were given. Pushing `ab` and then `cd`
// leaves `cdab` on the line.
func (e *editor) pushKeys(s string) { e.pushed = append([]byte(s), e.pushed...) }

// bell is what a terminal is asked to do about a search that found nothing.
// Both shells ring it, and it is the only report available: there is nowhere
// to put a sentence while a line is being typed.
const bell = "\a"

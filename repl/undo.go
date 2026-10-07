// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "slices"

// Taking a change back — `^_`, and `^X^U` for the same thing.
//
// It is the companion to a kill and the reason a kill is safe to press. `^Y`
// puts back what the last kill took, and it only covers a kill: a mistyped
// word, a completion that chose the wrong name, an `M-.` that walked one line
// too far are all changes `^Y` has nothing to say about. Without an undo the
// answer to every one of them is to retype the line.
//
// What is kept is the line *before* each change rather than a description of
// the change. A description would have to be written for each key and would
// then be wrong for the next one added; the line is the same shape whatever
// made it, and at the length of a command line copying it costs nothing next
// to the redraw that follows.
//
// The stack does not outlive the line. Measured, `^_` at a fresh prompt does
// nothing in both shells however much was edited on the line before it.

// snapshot is the line as it stood before one change, and where the cursor was
// in it.
type snapshot struct {
	line []rune
	pos  int
	// mark is no change at all: where insert mode was entered from command
	// mode, under EditorStyle.ViUndoAsReadline. See undo.
	mark bool
}

// change runs an editing key and remembers the line it found, so that `^_` can
// put it back.
//
// `continues` says this keystroke goes on with what the one before it was
// doing — a character typed after a character typed, or a further press of
// `M-.` walking the same insertion further back through the history. Where a
// dialect groups those into one change, the snapshot already on the stack
// covers this keystroke too and nothing is pushed.
//
// A key that left the line as it found it is not a change at all, and pushing
// for one would cost an undo that appears to do nothing and has to be pressed
// twice. So the line is compared afterwards rather than the key being asked in
// advance whether it will do anything: `^K` at the end of a line, `^W` at the
// start of one and a completion with no matches are all the same case, and
// each of them answering for itself is a place for one of them to forget.
func (e *editor) change(continues bool, edit func()) {
	was := snapshot{line: slices.Clone(e.line), pos: e.pos}
	edit()
	if slices.Equal(e.line, was.line) {
		return
	}
	if continues && !e.undoPerKeystroke && len(e.changes) > 0 {
		return
	}
	e.changes = append(e.changes, was)
}

// undo is the undo a key asks for: undoLine, unless that would take the line
// back past the limit a widget set (see Actions.UndoLimit).
//
// Measured 2026-10-04 through a pseudo-terminal against zsh 5.9.2: type `ab
// c`, a widget runs `UNDO_LIMIT_NO=$UNDO_CHANGE_NO`, type `d e`, and `^_`
// walks back to `ab c` and no further however often it is pressed. A limit
// past the current change stops every undo (#5898).
func (e *editor) undo() {
	if e.undoLimit > 0 && len(e.changes)+1 <= e.undoLimit {
		return
	}
	if e.viUndoReadline {
		// A mark: `^_` in insert mode stops at it once, with the bell, and
		// `u` in command mode passes over it. See EditorStyle.ViUndoAsReadline.
		n := len(e.changes)
		if n > 0 && e.changes[n-1].mark && e.viEditing() && !e.viCommand {
			e.changes = e.changes[:n-1]
			e.ring()
			return
		}
		for n > 0 && e.changes[n-1].mark {
			n--
		}
		e.changes = e.changes[:n]
	}
	if len(e.changes) == 0 && e.undoRings {
		// Nothing left to take back. See EditorStyle.UndoRingsWithNothingToUndo.
		e.ring()
		return
	}
	e.undoLine()
}

// undoLine puts the line back as it was before the last change.
func (e *editor) undoLine() {
	n := len(e.changes)
	if n == 0 {
		// Nothing has changed on this line. Measured, both shells leave it
		// alone rather than reaching back to the line before.
		return
	}
	was := e.changes[n-1]
	e.changes = e.changes[:n-1]
	e.pos = e.cursorAfterUndo(was)
	e.line = was.line
}

// cursorAfterUndo is where the cursor lands, which the dialects answer
// differently.
//
// zsh puts it back where it was when the change was made. bash puts it after
// the text the undo has just put back — which for a change that only removed
// text is the place the removal happened, and is why `^A`, `^K`, `^_` leaves
// bash at the end of the line and zsh at the start of it.
//
// bash's answer is computed rather than recorded: the restored text is what
// lies between the common prefix and the common suffix of the line as it is
// and the line as it was, and the cursor goes at the far end of it. That
// describes every measured case but one — `^T`, which bash records as two
// changes rather than one, so its undo leaves the cursor a character earlier
// than this. A swap is neither an insertion nor a removal, and it is not worth
// a second field to say so.
func (e *editor) cursorAfterUndo(was snapshot) int {
	if e.undoRestoresCursor {
		return was.pos
	}
	prefix := 0
	for prefix < len(e.line) && prefix < len(was.line) && e.line[prefix] == was.line[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(e.line)-prefix && suffix < len(was.line)-prefix &&
		e.line[len(e.line)-1-suffix] == was.line[len(was.line)-1-suffix] {
		suffix++
	}
	return len(was.line) - suffix
}

// changeNumber is the number of the change the line is in, counted so that
// the first change made to a line is 2: measured 2026-10-04 against zsh
// 5.9.2, `UNDO_CHANGE_NO` read in a widget after typing `xy` is 2, after a
// `BUFFER=one` 3, and after a further `LBUFFER+=two` 4 — one more than the
// changes before it, the typing counting once.
//
// The value is a token rather than a count anything should read as one: what
// a script does with it is hand it back to undoTo, which is the only reading
// that has to agree.
func (e *editor) changeNumber() int { return len(e.changes) + 1 }

// undoTo takes back every change made since change n, and reports false when
// n is before the first change — the line then goes back to how it began.
//
// Measured 2026-10-04 against zsh 5.9.2 through a pseudo-terminal, a widget
// on the line `xy`, numbering its own changes as changeNumber does:
//
//	a=$UNDO_CHANGE_NO; BUFFER=one; c=$UNDO_CHANGE_NO; LBUFFER+=two
//	zle .undo $c      0, `one`, cursor 2
//	zle .undo $a      0, `xy`
//	zle .undo 0       1, the empty line
//	zle .undo 99      0, the line as it was
//
// A number past the current change does nothing here. zsh keeps what an undo
// took back and walks forward to such a number, which this stack does not
// keep — the one difference, and a number only a redo could make reachable.
//
// Through undoLine one change at a time, so a numbered undo leaves the cursor
// where the plain one would.
func (e *editor) undoTo(n int) bool {
	keep := n - 1
	if keep < 0 {
		for len(e.changes) > 0 {
			e.undoLine()
		}
		return false
	}
	for len(e.changes) > keep {
		e.undoLine()
	}
	return true
}

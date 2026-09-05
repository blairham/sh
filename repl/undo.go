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

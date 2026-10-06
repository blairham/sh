// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "slices"

// ErrEditAgain is a read ended so that the whole command can be edited as one
// line: push-line-or-edit at a continuation prompt. The lines already entered
// are taken back, and the next read starts from them and the line that was
// being typed — see editor.editAgainText.
var ErrEditAgain = editAgainError{}

type editAgainError struct{}

func (editAgainError) Error() string { return "edit again" }

// pushLine puts the line aside and empties it: zsh's `push-line`. The next
// read at the main prompt starts from the line put aside last.
//
// Measured 2026-10-04 through a pseudo-terminal against zsh 5.9.2, from a
// widget: `echo y` put aside comes back on the prompt after the next command;
// two put aside come back newest first, one per prompt, and an empty line
// accepted is a prompt like any other (#5931).
//
// **The cursor comes back where it was, and the line comes back as a change.**
// Measured 2026-10-06 the same way, from `M-q` and from a widget that sets
// `CURSOR=3` first: `echo abc def` put aside with the cursor at 7 comes back
// with it at 7, and at 3 from the widget; and `^_` on the line that came back
// empties it (#6241). The first measurement pushed from the end of the line,
// where the two readings agree.
func (e *editor) pushLine(prompt drawnPrompt) {
	e.bufferStack = append(e.bufferStack, snapshot{line: slices.Clone(e.line), pos: e.pos})
	e.change(false, func() { e.line, e.pos = e.line[:0], 0 })
	e.redraw(prompt)
}

// acceptAndHold runs the line and hands it back at the next prompt: zsh's
// `accept-and-hold`, which is push-line and Return in one.
//
// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2, `zsh -f`,
// a two-row prompt: `M-a` on `echo abc def` with the cursor on the `c` prints
// `abc def`, and the next prompt holds `echo abc def` with the cursor on the
// `c` again; `^_` there empties it, as it does a line push-line put aside
// (#6241).
func (e *editor) acceptAndHold() {
	e.bufferStack = append(e.bufferStack, snapshot{line: slices.Clone(e.line), pos: e.pos})
	e.acceptRequested = true
}

// quoteLine quotes the whole line as one word: zsh's `quote-line`.
//
// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2: `echo
// it's x` becomes `'echo it'\”s x'` with the cursor at the end, an empty line
// becomes `”`, a backslash is left as it is, and pressing it again quotes
// the quoted line (#6241).
func (e *editor) quoteLine() {
	quoted := make([]rune, 0, len(e.line)+2)
	quoted = append(quoted, '\'')
	for _, r := range e.line {
		if r == '\'' {
			quoted = append(quoted, '\'', '\\', '\'', '\'')
			continue
		}
		quoted = append(quoted, r)
	}
	quoted = append(quoted, '\'')
	e.line, e.pos = quoted, len(quoted)
}

// pushLineOrEdit is push-line at the main prompt, and at a continuation
// prompt the command so far pulled back into one line.
//
// Measured the same way: with `if true; then` entered and `echo x` typed,
// the read ends — the typed line is wiped, the row ended — and the next one
// starts at the main prompt holding `if true; then⏎echo x`, the cursor at
// its end and `$PREBUFFER` empty. Return on a command that still is not
// finished then continues it as before. The read is ended at the top of the
// key loop, where every key's path comes back, so a widget that asked for it
// returns first.
func (e *editor) pushLineOrEdit(prompt drawnPrompt) {
	var before string
	if e.prebuffer != nil {
		before = e.prebuffer()
	}
	if before == "" {
		e.pushLine(prompt)
		return
	}
	e.editAgain = true
	e.editAgainText = before + string(e.line)
}

// editTheWholeCommand ends the read for push-line-or-edit: the line typed so
// far is wiped from the screen, as zsh wipes it, and the row is ended.
func (e *editor) editTheWholeCommand(prompt drawnPrompt) (string, error) {
	e.editAgain = false
	e.line, e.pos = e.line[:0], 0
	e.redraw(prompt)
	e.endLine(prompt, "")
	return "", ErrEditAgain
}

// popPushedLine starts a prompt's read from the line pushLine put aside
// last, if there is one and nothing else is starting it, with the cursor
// where it was when the line was put aside.
func (e *editor) popPushedLine() {
	n := len(e.bufferStack)
	if n == 0 || e.lineStart.seeded {
		return
	}
	held := e.bufferStack[n-1]
	e.line = append(e.line, held.line...)
	e.pos = min(held.pos, len(e.line))
	e.bufferStack = e.bufferStack[:n-1]
	e.poppedLine = true
}

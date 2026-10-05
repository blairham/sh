// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

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
// widget: `echo y` put aside comes back on the prompt after the next command,
// with the cursor at its end; two put aside come back newest first, one per
// prompt, and an empty line accepted is a prompt like any other (#5931).
func (e *editor) pushLine(prompt drawnPrompt) {
	e.bufferStack = append(e.bufferStack, string(e.line))
	e.change(false, func() { e.line, e.pos = e.line[:0], 0 })
	e.redraw(prompt)
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
// last, if there is one and nothing else is starting it.
func (e *editor) popPushedLine() {
	n := len(e.bufferStack)
	if n == 0 || e.lineStart.seeded {
		return
	}
	e.line = append(e.line, []rune(e.bufferStack[n-1])...)
	e.pos = len(e.line)
	e.bufferStack = e.bufferStack[:n-1]
}

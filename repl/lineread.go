// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"errors"
	"io"

	"github.com/blairham/sh/interp"
)

// The editor, handed to a command that asked for it.
//
// editoractions.go is the seam a *widget* reaches: the editor is already
// reading a key, and what runs is one of its own actions. This is the other
// direction and a different moment — a command is running, the editor is idle
// between two of its own reads, and the command wants a line of its own with
// text already on it.
//
// # The two things that stood in the way, and what each needed
//
// **The terminal is in its own line discipline while a command runs.**
// Shell.inLineDiscipline restores it before every command and puts raw mode
// back after, deliberately: a command may want echo, may want `^C` to reach it
// as a signal, and will print lines the terminal has to translate. So a read
// from inside a command is that helper run inside out — raw mode taken again
// for the length of the read and handed straight back.
//
// **editor.readLine clears the line it is given.** It resets the line, the
// cursor, the draft table, the undo stack and the history cursor at the top,
// which is right for a prompt and is exactly what this must not have: the text
// the command supplied *is* the line it starts from. lineStart is that, and it
// is a field rather than a parameter because a parameter would reach one
// caller through every test that drives a read.

// lineStart is what a read begins from, where it does not begin from nothing.
//
// The zero value is a prompt's read: an empty line, and an end-of-input key
// that ends the session. seeded is what tells the two apart, and it is its own
// field rather than "text is not empty" because a command may hand over an
// empty line and still mean this kind of read — creating a variable from
// whatever is typed is exactly that.
type lineStart struct {
	seeded bool
	text   string

	// endOnEndOfInput makes the end-of-input key on an empty line end the
	// read. Without it the key does what it does with a character under the
	// cursor, which on an empty line is nothing.
	//
	// **Measured, and the zsh answer is a third thing this editor has not
	// got.** 2026-09-18 through a pseudo-terminal against zsh 5.9.2:
	// `vared v` with the line emptied and `^D` pressed offered to list all
	// 1064 commands, because `^D` there is `delete-char-or-list` and this
	// editor's `^D` is not. With `-e` the same keystroke ended the read at
	// status 1 with the variable unchanged. So the option is measured and the
	// listing is a separate question — see repl.WidgetDeleteCharOrList, which
	// is the action, and the note beside this editor's `^D`.
	endOnEndOfInput bool
}

// readValue reads one line that starts from text the caller supplied.
//
// The history is the session's only where the request asked for it, and it is
// taken away rather than filtered: measured, Up during such a read rings the
// bell without the option and recalls the session's own last command with it,
// so what changes is whether there is a history at all.
func (e *editor) readValue(prompt drawnPrompt, req interp.LineEdit) (string, error) {
	e.lineStart = lineStart{seeded: true, text: req.Initial, endOnEndOfInput: req.EndOnEndOfInput}
	history, browsing := e.history, e.browsing
	if !req.History {
		e.history = nil
	}
	// And the word the session leaves with is not this read's to write. ^D
	// here ends the read the command asked for and the command goes on
	// running — the session is not ending, so there is nothing to say about
	// it, and the row is ended the way it is for a dialect that has no word.
	// Taken away and given back for the reason the history is: the next read
	// is the prompt's again. See editor.stopped.
	leaving := e.leaving
	e.leaving = ""
	defer func() {
		e.lineStart = lineStart{}
		e.history, e.browsing = history, browsing
		e.leaving = leaving
	}()
	return e.readLine(prompt)
}

// lineReader is the hook a session fills [interp.Runner.EditLine] in with.
//
// Held as a closure over the editor and the shell rather than reached through
// a field, because everything it needs is what Run already has in hand and
// nothing outside one read should be able to get at the editor.
func (s Shell) lineReader(ed *editor) func(interp.LineEdit) (string, interp.LineEditEnd) {
	return func(req interp.LineEdit) (string, interp.LineEditEnd) {
		// Raw mode for the length of the read and the terminal's own
		// discipline back afterwards, which is inLineDiscipline inverted —
		// see the file comment. The state captured here is the *command's*
		// terminal and not the session's, which is what makes handing it back
		// correct: the command goes on running after this returns.
		state, err := makeRaw(s.inFile())
		if err != nil {
			s.errf("%v\n", err)
			return "", interp.LineEditUnavailable
		}
		defer func() {
			if err := state.restore(); err != nil {
				s.errf("%v\n", err)
			}
		}()
		// Whatever the command printed before asking is no longer what the
		// terminal last saw, exactly as it is not after a command runs — see
		// crlf.forget, and inLineDiscipline, which does this for the same
		// reason on the way out.
		s.forgetWhatTheTerminalSaw()
		line, err := ed.readValue(drawPrompt(req.Prompt), req)
		switch {
		case errors.Is(err, ErrInterrupted):
			return "", interp.LineEditInterrupted
		case errors.Is(err, io.EOF):
			return "", interp.LineEditEndOfInput
		case err != nil:
			s.errf("%v\n", err)
			return "", interp.LineEditUnavailable
		}
		return line, interp.LineEditAccepted
	}
}

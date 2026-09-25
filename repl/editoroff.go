// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"io"
	"strings"
)

// editorIsOff reports whether this shell's line editor is turned off right
// now, which is a question one dialect's option table can answer and the rest
// cannot.
//
// A name the shell has never heard of is **not** a name that is off, which is
// the rule [Shell.commentsAreOff] states and the reason
// [interp.Runner.DialectOption] has a second result at all: reading an unknown
// name as off would take the line editor away from every dialect that never
// installed the option, and the failure would look like an editor that
// stopped working in a shell whose option table has nothing to say about one.
func (s Shell) editorIsOff() bool {
	name := s.Editor.RunsUnderTheOption
	if name == "" || s.Runner == nil {
		return false
	}
	on, known := s.Runner.DialectOption(name)
	return known && !on
}

// readWithoutTheEditor reads one line with the terminal in its own line
// discipline, which is what a session whose editor is turned off does.
//
// The terminal echoes the keystrokes, gathers the line and returns it on the
// newline, so there is nothing for this shell to draw: no ground cleared under
// the prompt, no bracketed-paste request, no redraw per keystroke. That is the
// whole of what `+Z` buys, and it is measured — see
// [EditorStyle.RunsUnderTheOption].
//
// The prompt is written **before** the terminal is handed back, through the
// stream the session wrapped rather than straight at the terminal. Raw mode is
// where the newline translation lives, and what it writes for a newline is
// byte for byte what the terminal's own discipline would have written, so a
// two-row prompt comes out the same either way — where writing it *after* the
// restore would put the return in twice, once here and once in the kernel.
//
// Everything else the session does stays exactly as it is: the interrupt is
// still caught, the history is still the editor's list, the prompt hooks still
// fire, and a construct still spans as many lines as it needs. Only the read
// is different.
func (s Shell) readWithoutTheEditor(state *terminalState, ed *editor, prompt drawnPrompt) (string, error) {
	s.errf("%s%s", prompt.lead, prompt.text)
	var (
		line string
		err  error
	)
	// inLineDiscipline and not a restore written here: it is the same handover
	// a command gets, and it is what puts raw mode back and forgets what the
	// translation thought the terminal last saw — which the echo of the line
	// this just read has invalidated.
	s.inLineDiscipline(state, func() {
		// A finished job says so before the read blocks, where the dialect
		// reports one the moment it ends — the same wake the editor's own
		// loop looks at, looked at from the one other place this session
		// waits. Nothing at all in the four dialects that hold the notice
		// for the next prompt. See jobnotify.go.
		ed.awaitFinishedJobs()
		line, err = readCookedLine(ed)
	})
	if ed.lineStart.seeded {
		// A command handed a line over — `vared`, or a verified history
		// expansion — and there is no editing line to draw it on, so it is
		// joined to what was typed. The same thing runPlain does with a seed,
		// and for the same reason.
		line = ed.lineStart.text + line
	}
	return line, err
}

// readCookedLine reads one line from a terminal that is gathering lines
// itself.
//
// **Through the editor's own reader and not the session's stream**, though
// there is no editor running: the editor holds whatever its last read took
// beyond the line it needed, and an option that moves between one line and the
// next moves *across* that buffer in both directions. Reading the descriptor
// underneath would lose every byte the editor had already been handed, and
// filling a buffer of this function's own would hide the same bytes from the
// editor one `setopt zle` later. There is one place the session's input is
// held, and this is it — see [editor.nextByte], whose comment names the two
// other places that made this mistake.
//
// A byte at a time, which costs nothing here: the bytes are already in hand,
// and a terminal in its own discipline hands over a line per read anyway.
//
// End of input is [io.EOF], which is what the session's ^D means and what the
// loop above already knows how to end on. A read that fails for any other
// reason is the same answer: the terminal is gone, and a session that cannot
// read is over.
//
// A final line the input ran out inside is still a line — the terminal returns
// it short of its newline, exactly as a ^D on a half-typed line does — so it
// is returned, and the read after it is the one that ends the session.
func readCookedLine(ed *editor) (string, error) {
	var b strings.Builder
	var one [1]byte
	for {
		// The error is dropped rather than carried: nextByte reports one only
		// where it had no byte to give, and every one of those ends this
		// session the same way the end of input does.
		if n, _ := ed.nextByte(one[:]); n <= 0 {
			if b.Len() > 0 {
				return b.String(), nil
			}
			return "", io.EOF
		}
		if one[0] == '\n' {
			return b.String(), nil
		}
		b.WriteByte(one[0])
	}
}

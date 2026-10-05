// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"errors"
	"io"
	"strings"
	"syscall"

	"github.com/blairham/sh/internal/tty"
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
//
// With several names the editor runs while any of them is on, and is off only
// when every one is known and off — bash's, which runs under `emacs` and `vi`
// and is turned off by `set +o emacs +o vi`.
func (s Shell) editorIsOff() bool {
	names := s.Editor.RunsUnderTheOptions
	if len(names) == 0 || s.Runner == nil {
		return false
	}
	for _, name := range names {
		if on, known := s.Runner.DialectOption(name); on || !known {
			return false
		}
	}
	return true
}

// readWithoutTheEditor reads one line with the terminal in its own line
// discipline, which is what a session whose editor is turned off does.
//
// The terminal echoes the keystrokes, gathers the line and returns it on the
// newline, so there is nothing for this shell to draw: no ground cleared under
// the prompt, no bracketed-paste request, no redraw per keystroke. That is the
// whole of what `+Z` buys, and it is measured — see
// [EditorStyle.RunsUnderTheOptions].
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
// is different — and so is what a ^C does to it, which is the terminal's to
// deliver here and not the editor's to read. See interruptedWhileReading.
func (s Shell) readWithoutTheEditor(state *terminalState, ed *editor, prompt drawnPrompt, sig *interrupts) (string, error) {
	// The interrupt character is made a byte first — before the prompt is
	// written, because a ^C typed the moment the prompt appears would
	// otherwise be the signal again, and before the job-notice wait below,
	// which is a wait for a whole line. See readCookedLineAtThePrompt. The
	// terminal is already in its own discipline here: the loop handed it over
	// before the prompt hooks ran.
	intr, restore := interruptAsAByte(state)
	defer restore()
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
		line, err = s.readCookedLineAtThePrompt(ed, intr)
	})
	if ed.lineStart.seeded {
		// A command handed a line over — `vared`, or a verified history
		// expansion — and there is no editing line to draw it on, so it is
		// joined to what was typed. The same thing runPlain does with a seed,
		// and for the same reason.
		line = ed.lineStart.text + line
	}
	if err == nil && s.interruptedWhileReading(ed, sig) {
		return "", ErrInterrupted
	}
	return line, err
}

// readCookedLineAtThePrompt is readCookedLine with the interrupt character
// made a byte this shell reads, so that a ^C at the prompt is answered here.
//
// Left to the terminal, ^C is SIGINT: the discipline throws away what was
// typed and goes on gathering, the read ends only on the next newline, and the
// handler the session installed for running commands is what hears it. Left
// there, the arrival waited for the next command and killed it (#5890) — and
// asking that handler after the read is a race, because the signal reaches it
// through two goroutines and the newline can arrive first. So for the length
// of the read the character ends the line as data instead: see
// [tty.InterruptEndsTheLine].
//
// Measured 2026-10-04 through a pseudo-terminal against zsh 5.9.2, `-f -i`,
// `unsetopt zle`: after `abc` and ^C, zsh writes nothing more than the
// terminal's own `^C`, and the line typed next — up to its newline — is given
// up whole and never run. The prompt then comes back fresh, a construct
// waiting at PS2 is abandoned with it, and `$?` is 130. Under `trap "" INT`
// the ^C throws away what was typed before it and the line typed after it
// runs, with `$?` left alone. Under a trap with a body zsh runs the trap, and
// its action's return decides the same two ways: `TRAPINT() { return 0 }`
// keeps reading, `return 130` gives the rest of the read up and leaves 130.
// The answer is the session's, the same one the editor's ^C gets — see
// Shell.answerInterrupt.
//
// bash, whose readline is off under `set +o emacs +o vi`, ends the read at the
// ^C instead: measured on bash 5.3.20, `abc` and ^C draw `^C`, a newline and
// a fresh prompt at once, and the line typed next runs. See
// EditorStyle.InterruptWithoutTheEditorTakesTheNextLine.
func (s Shell) readCookedLineAtThePrompt(ed *editor, intr byte) (string, error) {
	for {
		line, err := readCookedLine(ed, intr)
		if !errors.Is(err, errInterruptTyped) {
			return line, err
		}
		if ed.answerInterrupt == nil || ed.answerInterrupt() {
			// Kept — an ignore, or a trap that keeps the line. What the
			// terminal's discipline does with a ^C the shell does not give
			// the line up for: what was typed before it is thrown away and
			// the read goes on.
			continue
		}
		if !s.Editor.InterruptWithoutTheEditorTakesTheNextLine {
			// The read ends at the ^C: the terminal echoed `^C` where the
			// cursor was, and the newline is this shell's to write.
			s.errf("\n")
			return "", ErrInterrupted
		}
		// And the rest of the read is given up with it, to its newline — or
		// to a ^D, which ends the read it is given up with and not the
		// session: measured, zsh draws a fresh prompt after `abc`, ^C, ^D
		// and `$?` is 130. A terminal that has really gone answers the end
		// of input again at the next read, and that one ends the session.
		_, _ = readCookedLine(ed, 0)
		return "", ErrInterrupted
	}
}

// interruptAsAByte arms [tty.InterruptEndsTheLine] on the session's terminal
// and answers the character and how to put the terminal back. Zero, and
// nothing to put back, where there is no terminal or it cannot be changed —
// interruptedWhileReading is that route's answer.
func interruptAsAByte(state *terminalState) (byte, func()) {
	if state == nil {
		return 0, func() {}
	}
	mode, intr, err := tty.InterruptEndsTheLine(state.f)
	if err != nil {
		return 0, func() {}
	}
	return intr, func() { _ = mode.Restore() }
}

// errInterruptTyped is readCookedLine's answer for a line the interrupt
// character ended.
var errInterruptTyped = errors.New("interrupt typed")

// interruptedWhileReading reports whether a ^C arrived as a signal while the
// terminal was gathering the line just read, so that the line is given up
// rather than run.
//
// The route where readCookedLineAtThePrompt could not make the character a
// byte: a terminal it could not change. Both flags are taken, whatever the answer: the interpreter's, so no
// command is killed by an arrival that belonged to the read, and the prompt's,
// so no newline is written for a ^C the terminal's own echo of the newline
// already moved past.
func (s Shell) interruptedWhileReading(ed *editor, sig *interrupts) bool {
	if sig == nil || !sig.take() {
		return false
	}
	sig.took()
	// A trap of either kind is the signal's to answer: interp heard it too.
	if s.Runner.TrapsSignal()(syscall.SIGINT) {
		return false
	}
	return ed.answerInterrupt == nil || !ed.answerInterrupt()
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
//
// intr, where it is not zero, is a byte that ends the line as the newline does,
// answered with errInterruptTyped; see readCookedLineAtThePrompt.
func readCookedLine(ed *editor, intr byte) (string, error) {
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
		if intr != 0 && one[0] == intr {
			return b.String(), errInterruptTyped
		}
		b.WriteByte(one[0])
	}
}

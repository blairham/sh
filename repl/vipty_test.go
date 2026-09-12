// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"testing"
	"time"
)

// vi command mode through a real terminal, in a real session.
//
// The table in vi_test.go asks what the editor does with a keystroke and says
// nothing about whether the keystroke reaches it: it hands the editor a reader,
// and from there a key that never arrives looks exactly like a key that works.
// The mode is only reachable where there is a terminal, and one thing about it
// is *only* answerable here — whether a bare Escape is told apart from the
// first byte of an arrow key, which turns on how the bytes arrive rather than
// on what they are.

// viSession is a session whose front end says this shell edits the vi way,
// which is the only thing that gives it a command mode.
func viSession(t *testing.T) *session {
	t.Helper()
	return newSessionWith(t, func(s *Shell) { s.ViEditing = func() bool { return true } })
}

// TestTheCommandModeThroughATerminal is the mode end to end: Escape leaves
// insert, the motions and the operator run, and Return accepts from command
// mode without going back to insert first.
//
// **What ran is compared whole rather than searched**, and that is not
// fussiness. An Escape that was *not* acted on leaves the keys after it to be
// typed, so the line becomes the one that was wanted with the keystrokes stuck
// on the end of it — and a test that looked for its answer inside the output
// would find it there and pass.
func TestTheCommandModeThroughATerminal(t *testing.T) {
	s := viSession(t)
	// Escape, to the start, a word forward, and the word deleted — then
	// Return straight from command mode.
	s.typeLine("echo junk one two\x1b0wdw\n")
	s.ranExactly("one two\n")
	// And back to insert mode, where an ordinary letter is a letter again.
	s.typeLine("cho three\x1b0ie\n")
	s.ranExactly("one two\nthree\n")
	s.end()
}

// ranExactly waits for the commands to have printed exactly this much and
// nothing else.
//
// The wait is what makes it usable — output arrives after the keystroke — and
// the equality is what makes it evidence: see the note on the test above.
func (s *session) ranExactly(want string) {
	s.t.Helper()
	for waited := 0; waited < 4000; waited++ {
		if s.ran.String() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	s.t.Fatalf("the commands printed %q, want %q", s.ran.String(), want)
}

// TestAViCommandModeLineIsFollowedByATypingPrompt — the mode belongs to the
// line, so the prompt after one accepted from command mode takes typing.
func TestAViCommandModeLineIsFollowedByATypingPrompt(t *testing.T) {
	s := viSession(t)
	// `0dwiecho ` is only meaningful in command mode: the operator takes the
	// first word off and `i` puts the editor back into insert to type a new
	// one. Then Return, from command mode again.
	s.typeLine("junk first\x1b0dwiecho \x1b0\r")
	s.ranExactly("first\n")
	s.typeLine("echo second\n")
	s.ranExactly("first\nsecond\n")
	s.end()
}

// TestAnArrowKeyStillWorksInViMode is the one claim no reader-driven test can
// make.
//
// Escape is the mode switch and also the first byte of every arrow key, and
// both real shells tell them apart with a timer — measured, `\e[D` typed as one
// burst moves the cursor left in bash 5.3.15 and zsh 5.9.2, and the same three
// bytes with 1.2 seconds after the Escape leave insert mode and are read as two
// command-mode keys. This editor asks whether a byte is *there* instead, which
// is a question about how the bytes arrived: a terminal writes a key sequence
// in one write and a person pressing Escape writes one byte.
//
// So the two halves are one write and three writes with real time between
// them, and the difference has to show up in what the shell runs.
func TestAnArrowKeyStillWorksInViMode(t *testing.T) {
	s := viSession(t)
	s.typeLine("echo onetwo")
	// The whole sequence in one write, which is what a terminal does with a
	// key. It moves the cursor left; the space then splits the word.
	if _, err := s.control.WriteString("\x1b[D"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.control.WriteString(" \n"); err != nil {
		t.Fatal(err)
	}
	s.ranExactly("onetw o\n")
	s.end()
}

// TestABareEscapeLeavesInsertAtOnce is the other half: an Escape with nothing
// behind it is the mode switch, and it does not wait for a key that is never
// coming.
//
// The wait is what an emacs-mode Escape does — see
// TestAnEscapeWaitsForTheKeyItNames, and the measurement that all three shells
// wait indefinitely for it. In vi mode waiting would be waiting to find out
// whether a key that already means something meant something else.
func TestABareEscapeLeavesInsertAtOnce(t *testing.T) {
	s := viSession(t)
	s.typeLine("echo one twoX\x1b")
	// Long enough that a wait for the next byte would still be waiting, and
	// long enough for the redraw the mode switch causes to have happened.
	time.Sleep(300 * time.Millisecond)
	// `x` is only a command-mode key. In insert mode it would be typed.
	if _, err := s.control.WriteString("x\n"); err != nil {
		t.Fatal(err)
	}
	s.ranExactly("one two\n")
	s.end()
}

// TestWithoutViEditingATerminalSessionIsUnchanged is the regression guard at
// the level a person would notice: a session that never asked for vi editing
// reads Escape exactly as it always did.
func TestWithoutViEditingATerminalSessionIsUnchanged(t *testing.T) {
	s := newSession(t)
	s.typeLine("echo one two\x1bbX\n")
	s.ranExactly("one Xtwo\n")
	s.end()
}

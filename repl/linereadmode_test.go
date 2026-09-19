// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package repl

import (
	"io"
	"syscall"
	"testing"

	"github.com/blairham/sh/interp"
)

// The line discipline around a read a command asked for.
//
// Asked of the terminal rather than of the bytes, for the reason
// TestWorkHandedTheTerminalBackFindsItsOwnLineDiscipline gives: this is the
// state the *next* thing to print will find, and the bytes only show it once
// something has printed two lines. The session fixture cannot show it at all —
// a command's output there does not go to the terminal.
//
// Both halves are the assertion, and the second is the one the pty tests
// cannot reach. Taking raw mode is what makes the read work; handing it back
// is what keeps the rest of the command running on the terminal it was given.
func TestAReadFromACommandTakesRawModeAndHandsItBack(t *testing.T) {
	control, terminal := openTerminal(t)

	// A fresh terminal is in its own line discipline, which is what
	// inLineDiscipline leaves behind before a command runs.
	if !echoes(t, terminal) {
		t.Fatal("the fixture did not start in the terminal's own line discipline")
	}

	var during bool
	s := Shell{
		In: terminal, Out: io.Discard, Err: io.Discard,
		// The completer runs while the editor is reading, which is the one
		// hook this package has that is called from inside a read — so it is
		// where the mode during the read can be asked.
		Completers: []Completer{CompleterFunc(func(Completion) []Candidate {
			termios, err := probeTermios(int(terminal.Fd()))
			during = err == nil && termios.Lflag&syscall.ECHO == 0
			return nil
		})},
	}
	ed := s.newEditor(t.Context(), nil)
	if _, err := control.WriteString("\t\n"); err != nil {
		t.Fatal(err)
	}
	line, end := s.lineReader(ed)(interp.LineEdit{Initial: "x"})

	if !during {
		t.Error("the read ran with the terminal still in its own line discipline")
	}
	if !echoes(t, terminal) {
		t.Error("the terminal was not handed back — the rest of the command runs in raw mode")
	}
	if line != "x" || end != interp.LineEditAccepted {
		t.Errorf("the read gave back %q, %d, want %q accepted", line, end, "x")
	}
}

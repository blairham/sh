// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"testing"

	"github.com/blairham/sh/internal/pty"
	"github.com/blairham/sh/interp"
)

// $LINES and $COLUMNS follow the window, where the shell has asked for it.
//
// The name bash gives this is `checkwinsize` and the name zsh gives it is no
// name at all, so what is asserted here is the capability and not either
// spelling: a Runner that says it tracks the window size has both variables
// set from the terminal, and one that does not has neither.
//
// A real pseudo-terminal, resized between the two questions, because the
// whole of the option is that the *second* answer differs from the first.
// Nothing short of a terminal can be resized, which is why this could not be
// a corpus row and why #1429 refused the name from a `-c` probe that showed
// bash and this shell both reporting the variables unset.
func TestWindowSizeTrackingSetsLinesAndColumns(t *testing.T) {
	control, terminal, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	t.Cleanup(func() {
		_ = terminal.Close()
		_ = control.Close()
	})
	if err := pty.SetSize(terminal, 24, 80); err != nil {
		t.Skipf("no resize: %v", err)
	}
	r := &interp.Runner{}
	s := Shell{Runner: r, In: terminal}

	// Nothing said, nothing assigned: the capability is off in a bare Runner,
	// and a session that assigned $COLUMNS anyway would be answering for the
	// two shells in the panel that never do.
	s.trackWindowSize()
	if v, ok := r.GetVar("COLUMNS"); ok {
		t.Errorf("COLUMNS = %q with tracking off, want unset", v)
	}
	if v, ok := r.GetVar("LINES"); ok {
		t.Errorf("LINES = %q with tracking off, want unset", v)
	}

	r.SetTracksWindowSize(true)
	s.trackWindowSize()
	if v, _ := r.GetVar("COLUMNS"); v != "80" {
		t.Errorf("COLUMNS = %q, want 80", v)
	}
	if v, _ := r.GetVar("LINES"); v != "24" {
		t.Errorf("LINES = %q, want 24", v)
	}

	// And they move with the window, which is the half a one-shot assignment
	// at startup would also pass.
	if err := pty.SetSize(terminal, 40, 132); err != nil {
		t.Fatalf("resize: %v", err)
	}
	s.trackWindowSize()
	if v, _ := r.GetVar("COLUMNS"); v != "132" {
		t.Errorf("COLUMNS after a resize = %q, want 132", v)
	}
	if v, _ := r.GetVar("LINES"); v != "40" {
		t.Errorf("LINES after a resize = %q, want 40", v)
	}
}

// Nothing is written for a size the terminal will not give.
//
// Zero is what terminalSize answers for a pipe, a closed terminal or a kernel
// that declines, and assigning `0` would be worse than assigning nothing: a
// prompt that wraps at $COLUMNS is broken by a zero in a way an unset variable
// does not break it. Measured the same way — bash with `-i` on a pipe leaves
// both unset.
func TestWindowSizeTrackingWritesNothingWithoutATerminal(t *testing.T) {
	r := &interp.Runner{}
	r.SetTracksWindowSize(true)
	// In is a reader that is not a file, so inFile is nil and the ioctl has
	// nothing to ask.
	s := Shell{Runner: r, In: readerOnly{}}
	s.trackWindowSize()
	if v, ok := r.GetVar("COLUMNS"); ok {
		t.Errorf("COLUMNS = %q with no terminal, want unset", v)
	}
	if v, ok := r.GetVar("LINES"); ok {
		t.Errorf("LINES = %q with no terminal, want unset", v)
	}
}

// readerOnly is an input that is not a descriptor.
type readerOnly struct{}

func (readerOnly) Read([]byte) (int, error) { return 0, nil }

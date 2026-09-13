// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"testing"

	"github.com/blairham/sh/internal/pty"
	"github.com/blairham/sh/internal/tty"
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
	_, terminal := openTerminalAt(t, 24, 80)
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

// A terminal that will not say how big it is writes nothing here, and that is
// the measured answer rather than an omission.
//
// A pseudo-terminal created without a window size answers `TIOCGWINSZ` with
// 0x0 — what `pty.fork()` produces, and what sits in the window between
// `forkpty` and the parent's `TIOCSWINSZ`. On exactly that terminal, bash 5.3
// under `-i` leaves `$COLUMNS` and `$LINES` **unset**, measured 2026-09-13,
// where zsh reports the classic 80 and 24 in the parameters it provides
// itself (#2489). This half is bash's, so it writes nothing.
//
// The two are not one rule with two spellings: interp.Runner.windowSizeValue
// is the other one, and it falls back where this declines to.
func TestAnUnsizedTerminalWritesNothingToTheVariables(t *testing.T) {
	_, terminal := openTerminalAt(t, 0, 0)
	r := &interp.Runner{}
	r.SetTracksWindowSize(true)
	s := Shell{Runner: r, In: terminal}
	s.trackWindowSize()
	if v, ok := r.GetVar("COLUMNS"); ok {
		t.Errorf("COLUMNS = %q, want unset — this half is bash's and bash writes nothing", v)
	}
	if v, ok := r.GetVar("LINES"); ok {
		t.Errorf("LINES = %q, want unset", v)
	}
}

// And the width the editor draws against is eighty there, which is the other
// half of the same terminal and the opposite answer.
//
// The row above is the *parameter*, which is bash's and writes nothing. This
// is the *drawing*, which has nowhere to decline to: there is a screen and
// something has to go on it, so a guess beats a refusal and eighty is the
// guess zsh makes. Taking the ioctl's zero literally is what switched off
// every piece of wrapping arithmetic the editor has, and it is why every pty
// fixture in this package was grading the wrong redraw (#2627).
//
// Zero by zero rather than a pipe, because a pipe would prove the wrong thing.
// The fallback is a statement about an unhelpful terminal and not about the
// absence of one — see internal/tty, where the constants and the measurement
// behind them live.
func TestATerminalThatWillNotSayItsWidthIsDrawnForAtEighty(t *testing.T) {
	_, terminal := openTerminalAt(t, 0, 0)
	if rows, cols := terminalSize(terminal); rows != 0 || cols != 0 {
		t.Fatalf("the terminal reported %dx%d; this test needs one that will not say", rows, cols)
	}
	if got := terminalWidth(terminal); got != tty.FallbackCols {
		t.Errorf("terminalWidth = %d for a terminal that will not say, want the %d fallback", got, tty.FallbackCols)
	}
}

// Nothing to ask is not the same as a terminal that will not answer: the
// fallback is for a terminal, and a session whose input is not one has no
// screen to guess the width of.
func TestSomethingThatIsNotATerminalHasNoWidthAtAll(t *testing.T) {
	if got := terminalWidth(nil); got != 0 {
		t.Errorf("terminalWidth(nil) = %d, want 0 — there is no terminal to fall back for", got)
	}
}

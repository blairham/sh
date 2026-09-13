// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/pty"
)

// The terminal every fixture in this package runs against.
//
// **A pseudo-terminal is born zero by zero**, and zero is a real answer from
// the size ioctl meaning "do not know". Every fixture here opened one and
// none of them said how big it was, so `editor.cols()` was 0 in all of them —
// measured 2026-09-13 at 94 calls to it in TestEditingThroughATerminal alone,
// every one answering 0 from a descriptor whose ioctl succeeded. Width 0 is
// its own branch of [editor.redraw]: the line is assumed to fit on the row it
// started on and the whole of it is written again. So the suite graded the
// whole-line draw, and [editor.repaint] — the O(change) path a session with a
// real terminal takes on every keystroke — was graded by unit tests over the
// function and by no session at all (#2627).
//
// Nothing warned, because a terminal that will not say its size is a case the
// editor handles rather than an error it reports. The fix is not a check; it
// is that there is now one way to open a terminal here and it has a size.
//
// 24x80 because it is the size a terminal has when nothing else decides, and
// because it is what internal/smoke's session already uses.
const (
	fixtureRows = 24
	fixtureCols = 80
)

// openTerminal opens a pseudo-terminal for a fixture, sized, closed on the way
// out.
//
// Twenty-two copies of the same seven lines preceded it — open, skip if the
// platform has none, close both ends — which is how twenty-two fixtures came
// to share one omission. Folding them leaves one place for the size to live
// and one place for a later fixture to inherit it from.
func openTerminal(t *testing.T) (control, terminal *os.File) {
	t.Helper()
	return openTerminalAt(t, fixtureRows, fixtureCols)
}

// openTerminalAt is the same terminal at a size the test chose, for the tests
// whose subject is the size.
func openTerminalAt(t *testing.T, rows, cols int) (control, terminal *os.File) {
	t.Helper()
	control, terminal, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	t.Cleanup(func() {
		_ = terminal.Close()
		_ = control.Close()
	})
	if err := pty.SetSize(terminal, rows, cols); err != nil {
		t.Skipf("no resize: %v", err)
	}
	return control, terminal
}

// editedLine is the line the editor is showing: the row of the screen the
// cursor is on, with the prompt still on the front of it.
//
// It exists because "what the editor is showing" stopped being a substring of
// what the editor wrote. The whole-line draw put the prompt and the line back
// on every keystroke, so a wait for `$ echo seeded` was answered by the bytes
// themselves; the redraw a session with a real terminal takes moves the cursor
// and overwrites — `\e[4Dseeded` — and the literal is never written at all.
// The screen says the same thing either way, which is why the wait moved here
// (#2627).
// Leading blanks come off, and that is the fixture's doing rather than the
// shell's. These sessions hand the Runner the same buffer they hand the editor
// rather than the terminal, so a command's output arrives as a bare `\n` — no
// carriage return, because nothing put one there — and the prompt after it
// starts in whichever column the output ended in. A real session's commands
// write to the terminal with its own line discipline on, where `\n` is `\r\n`
// and the prompt starts at column zero. The indent is therefore an artifact of
// how the output was captured, and the line after it is the subject.
func editedLine(out string) string {
	s := shownBy(fixtureCols, out)
	row, _ := s.at()
	rows := strings.Split(s.text(), "\n")
	if row >= len(rows) {
		return ""
	}
	return strings.TrimLeft(rows[row], " ")
}

// waitForLine waits until the line the editor is showing is exactly want.
//
// Exactly, and not a substring: the case these waits are for is the up arrow
// replacing a line with a shorter one, and a redraw that left the tail of the
// old line behind would satisfy a containment check while showing something
// nobody typed.
func waitForLine(t *testing.T, buf *syncBuffer, want, what string) {
	t.Helper()
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); {
		if editedLine(buf.String()) == want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("waiting for %s: the line being edited never read %q; it reads %q, and what was written was %q",
		what, want, editedLine(buf.String()), buf.String())
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/pty"
	"github.com/blairham/sh/internal/smoke"
)

// `+Z` — `unsetopt zle` — is an interactive shell with the line editor off,
// and it is how three of zsh's own suite files start the shell they drive
// through `zsh/zpty`: `-fiV +Z`. The reference then writes nothing to the
// terminal but what the commands print, because there is no editor to draw
// anything.
//
// Measured 2026-09-25 against zsh 5.9.2 (aarch64-apple-darwin25.4.0) through a
// pseudo-terminal with `TERM=dumb` and `PS1`/`PS2` exported empty, one command
// typed and then `exit`:
//
//	-fiV +Z   b': &\r\n[1] 6064\r\n[1]  + done       :\r\nexit\r\n'
//	-fiV      b' \r\x1b[?2004h: &\x1b[?2004l\r\r\n[1] 6090\r\n \r…'
//
// Both rows matter and the second is the control: this shell **does** write
// `\e[?2004h` where the reference does, so a test asserting only the absence
// would pass for a shell that had stopped asking for bracketed paste
// altogether. The failure being pinned is the first row — every line of those
// sessions wrapped in a redraw and a paste toggle, which is 79 differing lines
// over three files of the zsh column (#4472).
//
// The assertion is on the **raw bytes the terminal was handed**. Anything that
// strips escapes, or reads a rendered screen, cannot see this bug at all: what
// it is about is precisely the bytes a renderer throws away.
func TestTheLineEditorIsOffWhenTheOptionIs(t *testing.T) {
	for _, tc := range []struct {
		name string
		argv []string
		// drawing is whether the editor is expected to have drawn.
		drawing bool
	}{
		{
			name:    "with +Z the editor writes nothing",
			argv:    []string{"zsh", "-i", "+Z"},
			drawing: false,
		},
		{
			// The same session one letter apart, which is what says the row
			// above is about the option and not about a shell that draws
			// nothing.
			name:    "and without it the editor draws as it always has",
			argv:    []string{"zsh", "-i"},
			drawing: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Everything the terminal was given, read once the session has
			// finished writing — zleOffSession waits on the EXIT trap's
			// word, which is why the absence rows below are assertions
			// rather than a race the reader usually wins (#4579).
			screen := zleOffSession(t, tc.argv)
			drawn := screen.Text()
			// The command ran, which is the half that keeps the other half
			// honest: a session that drew no escapes because it never reached
			// a prompt would pass an assertion about escapes alone.
			if !strings.Contains(drawn, zleOffAnswer) {
				t.Fatalf("the command did not run:\n%s", smoke.Readable(smoke.LastLines(drawn, 8)))
			}
			for _, seq := range []struct{ name, text string }{
				{"the bracketed-paste request", "\x1b[?2004h"},
				{"the bracketed-paste release", "\x1b[?2004l"},
				{"the ground under the prompt", "\x1b[0m\x1b[27m\x1b[24m\x1b[J"},
			} {
				if got := strings.Contains(drawn, seq.text); got != tc.drawing {
					t.Errorf("%s written = %v, want %v\nraw bytes: %q",
						seq.name, got, tc.drawing, drawn)
				}
			}
		})
	}
}

// zleOffMark is the last row of the two-row prompt this session synchronizes
// on, and it is not text anybody types: a wait on a mark the typed line
// contains passes for a shell that echoed the line and ran nothing. With the
// editor off the terminal echoes what is typed, so that hazard is sharper here
// than anywhere else in this package.
const zleOffMark = "zoff> "

// zleOffAnswer is what the one command prints, written as an expression so
// that the echo of the line cannot be mistaken for the answer to it.
const zleOffAnswer = "zle-42-ok"

// zleOffEnd is written by an EXIT trap the startup file below sets, and is the
// last thing either session here writes. It is what both of them read the
// screen against.
//
// The shell's run returns when its last write is **issued** and not when the
// terminal has been read, so a row that reads `screen.Text()` straight after
// `<-done` reads whatever the reader goroutine happened to have reached. That
// is not a slow instrument, it is an unsynchronized one: with the reader
// wrapped so that it sleeps for one read after the read carrying the echo of
// the typed line, the editorless row below failed **three runs in three** and
// reported a line the shell had written and the reader had not yet appended
// (#4579). The same wrapper leaves the row above green, which is what says it
// discriminates rather than merely delaying everything.
//
// The mark is the trap's and not any sentence of the session's for the reason
// #4577 arrived at one file over: what is waited on has to be written *after*
// everything asserted on, or the wait orders nothing. The escape rows here
// assert **absences**, and an absence has nothing of its own to wait for —
// an early read turns it into a silent pass rather than a failure.
//
// Re-measured 2026-09-26 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — on a pseudo-terminal with `TERM=dumb` and
// this file's own two-row `PS1`, each of the two sessions run twice, once
// with the `trap` line in the startup file and once without. The bytes are
// identical up to the trap's own line, which comes last both times:
//
//	-i +Z, no trap    …zoff> echo zle-$((6 * 7))-ok\r\nzle-42-ok\r\n…zoff> exit\r\n
//	-i +Z, with trap  the same, then ZOFF-END-4579\r\n
//	-i, no trap       …zoff> \e[?2004hecho zle-$((6 * 7))-ok\e[?2004l\r\r\nzle-42-ok\r\n…
//	                  zoff> \e[?2004hexit\e[?2004l\r\r\n
//	-i, with trap     the same, then ZOFF-END-4579\r\n
//
// So the trap changes neither row, and its word falls after the editor's last
// escape — which is the property the absence rows need and no sentence of
// these sessions could give them. It is not text either session types, so a
// wait on it cannot be satisfied by the terminal's echo of the input.
const zleOffEnd = "ZOFF-END-4579"

// zleOffRC is the startup file both sessions in this file are given, so that
// no row here can be written that waits on the wrong line.
const zleOffRC = "PS1=$'ZOFFROW\\n" + zleOffMark + "'\n" +
	"trap 'print -r -- " + zleOffEnd + "' EXIT\n"

// zleOffBudget turns a hang into a failure.
const zleOffBudget = 20 * time.Second

// zleOffSession puts an interactive zsh on a pseudo-terminal with a **two-row**
// prompt and a scratch home, types one command and then `exit`, and hands back
// everything the shell wrote.
//
// Two rows and not one. A prompt whose upper row is rewritten on every
// keystroke draws a ladder of prompts down the screen and a one-row `PS1`
// cannot see it (#2467) — and a two-row prompt is also what catches the other
// half of this change: the prompt on the editor-less route is written before
// the terminal is handed back, so a newline in it that was translated twice
// would show up here and nowhere else.
func zleOffSession(t *testing.T, argv []string) *smoke.Screen {
	t.Helper()
	home := scratchHome(t)
	control, terminal, err := pty.Open()
	if errors.Is(err, pty.ErrUnsupported) {
		t.Skip("no pseudo-terminal on this platform")
	}
	if err != nil {
		t.Fatalf("opening a pseudo-terminal: %v", err)
	}
	if err := pty.SetSize(terminal, 24, 100); err != nil {
		t.Fatalf("sizing the terminal: %v", err)
	}
	writeHomeFile(t, home, ".zshrc", zleOffRC)

	sh := scratchShell(t)
	sh.Stdin, sh.Stdout, sh.Stderr = terminal, terminal, terminal
	sh.Dir = home
	sh.Env = []string{
		"HOME=" + home,
		"PATH=/usr/bin:/bin",
		"TERM=dumb",
		"HISTFILE=" + filepath.Join(home, "hist"),
	}
	screen := smoke.Watch(control)
	done := make(chan int, 1)
	go func() { done <- driver.MainArgs(sh, argv) }()
	t.Cleanup(func() {
		_ = terminal.Close()
		_ = control.Close()
	})
	if err := screen.Await(zleOffMark, zleOffBudget); err != nil {
		t.Fatalf("no first prompt: %v", err)
	}
	zleOffType(t, control, "echo zle-$((6 * 7))-ok")
	if err := screen.Await(zleOffMark, zleOffBudget); err != nil {
		t.Fatalf("no prompt after the command: %v", err)
	}
	// `exit` rather than closing the control end, so the session ends the way
	// a person ends one and the shell has written everything it is going to.
	zleOffType(t, control, "exit")
	select {
	case <-done:
	case <-time.After(zleOffBudget):
		t.Fatalf("the shell did not exit:\n%s", smoke.Readable(smoke.LastLines(screen.Text(), 8)))
	}
	// And then on the screen rather than on the process, which is a different
	// event: the run above returns when the shell's last write is issued and
	// this returns when the terminal has been read. See zleOffEnd.
	if err := screen.Await(zleOffEnd, zleOffBudget); err != nil {
		t.Fatalf("the shell never finished writing: %v\n%s",
			err, smoke.Readable(smoke.LastLines(screen.Text(), 8)))
	}
	return screen
}

func zleOffType(t *testing.T, control *os.File, line string) {
	t.Helper()
	if _, err := control.WriteString(line + "\n"); err != nil {
		t.Fatalf("typing %q: %v", line, err)
	}
}

// A line written before the session has drawn its first prompt still reaches
// it.
//
// This is how every `zpty`-driven suite file drives a shell: start it, write
// the line, read the answer — the write lands within microseconds of the
// child, long before it has anything to read with. It has to work, and the
// obvious way to hand the editor its terminal breaks it.
//
// Measured 2026-09-25 on macOS: a line that arrives while the line discipline
// is **off** is held in the terminal's raw queue and is *not* promoted to the
// canonical queue when canonical mode comes back. So a session that took raw
// mode at startup and handed it back for an editor-less read lost the line
// outright and blocked on a read that would never return — three `zpty` files
// of zsh's suite went from scoring to hanging, and a two-second pause before
// the write made all three pass again. That pause is what makes this a race
// rather than a shape, and why the test writes with no wait at all.
//
// The fix is that the mode is taken by the read that wants one: a `+Z` session
// never turns the discipline off, so there is no window for a line to fall
// into. See repl's terminalFor.
func TestALineWrittenBeforeTheFirstPromptReachesAnEditorlessSession(t *testing.T) {
	home := scratchHome(t)
	control, terminal, err := pty.Open()
	if errors.Is(err, pty.ErrUnsupported) {
		t.Skip("no pseudo-terminal on this platform")
	}
	if err != nil {
		t.Fatalf("opening a pseudo-terminal: %v", err)
	}
	if err := pty.SetSize(terminal, 24, 100); err != nil {
		t.Fatalf("sizing the terminal: %v", err)
	}
	writeHomeFile(t, home, ".zshrc", zleOffRC)

	sh := scratchShell(t)
	sh.Stdin, sh.Stdout, sh.Stderr = terminal, terminal, terminal
	sh.Dir = home
	sh.Env = []string{
		"HOME=" + home,
		"PATH=/usr/bin:/bin",
		"TERM=dumb",
		"HISTFILE=" + filepath.Join(home, "hist"),
	}
	screen := smoke.Watch(control)
	done := make(chan int, 1)
	go func() { done <- driver.MainArgs(sh, []string{"zsh", "-i", "+Z"}) }()
	t.Cleanup(func() {
		_ = terminal.Close()
		_ = control.Close()
	})
	// **No wait for the prompt**, which is the whole of the test: the line is
	// written while the shell is still starting, exactly as a driver writes
	// it. Both lines at once, so the session also ends without a second wait.
	zleOffType(t, control, "echo zle-$((6 * 7))-ok")
	zleOffType(t, control, "exit")
	select {
	case <-done:
	case <-time.After(zleOffBudget):
		t.Fatalf("the session never read the line it was written:\n%s",
			smoke.Readable(smoke.LastLines(screen.Text(), 8)))
	}
	// The shell has stopped running; the screen has not necessarily been read.
	// Waiting on the EXIT trap's word is what makes the reading below about
	// the line rather than about the reader — see zleOffEnd, and #4579 for
	// what it cost while this was `<-done` alone.
	if err := screen.Await(zleOffEnd, zleOffBudget); err != nil {
		t.Fatalf("the shell never finished writing: %v\n%s",
			err, smoke.Readable(smoke.LastLines(screen.Text(), 8)))
	}
	if drawn := screen.Text(); !strings.Contains(drawn, zleOffAnswer) {
		t.Fatalf("the line was lost:\n%s", smoke.Readable(smoke.LastLines(drawn, 8)))
	}
}

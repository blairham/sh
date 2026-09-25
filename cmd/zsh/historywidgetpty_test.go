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

// `$history` read from a **widget**, on a real terminal — which is the one
// reading this parameter exists for and the only one that can be measured
// here.
//
// zsh-autosuggestions' default strategy runs in a widget bound to every
// self-insert, and its whole body is `typeset -g suggestion=${history[(r)$~pattern]}`.
// Two things about that reading are invisible to a script and to a session on
// a pipe, and both were wrong before #4408:
//
//   - The table has to hold **the command just run**. zsh leaves out the
//     event `$HISTCMD` names, which while a command is running is that
//     command and at a prompt is the line being typed — so a widget sees
//     every command the session has run, and a view that dropped the newest
//     entry here would drop the most recent one, which is the one a
//     suggestion is usually made of. See zshHistoryEvents.
//   - `(r)` takes the **first** match in scan order, so the order of the keys
//     is the difference between suggesting the command from a minute ago and
//     the one from the top of the list. This engine's tables read in sorted
//     key order, which is that order backwards.
//
// Measured 2026-09-24 against zsh 5.9.2 (aarch64-apple-darwin25.4.0) under
// `zsh -f -i` on a pseudo-terminal, typing the same five lines this test types
// and then pressing the same ^G at an empty prompt:
//
//	WIDGET count=5 newest=echo BBB keys=5 4 3 2 1
//
// The count is five and not two because the three lines that *define* the
// widget are commands like any other — which is the part worth keeping, since
// it is what says the table holds the line run immediately before the
// keystroke.
//
// That is what this test requires. Against the commit before the fix the
// same keystroke writes `zz: history: parameter not implemented yet`, which
// is the line a person saw once per character.

// widgetMark is the last row of the prompt this session synchronizes on, and
// it is not text anybody types: a wait on a mark the typed line contains
// passes for a shell that echoed the line and ran nothing.
const widgetMark = "hw> "

// widgetBudget turns a hang into a failure.
const widgetBudget = 20 * time.Second

func TestAWidgetReadsTheHistoryTableAtAPrompt(t *testing.T) {
	control, screen := widgetSession(t)
	// The widget is written the way the plugin writes its own: one line, the
	// table searched by value, and the answer printed rather than drawn, so
	// what is asserted is the reading and not the redraw.
	widgetType(t, control, screen,
		`zz() { print -r -- "WIDGET count=${#history} newest=${history[(r)echo*]} keys=${(k)history}" }`)
	widgetType(t, control, screen, "zle -N zz")
	widgetType(t, control, screen, `bindkey '^G' zz`)
	widgetType(t, control, screen, "echo AAA")
	widgetType(t, control, screen, "echo BBB")

	before := screen.Text()
	if _, err := control.WriteString("\a"); err != nil {
		t.Fatalf("pressing ^G: %v", err)
	}
	if err := screen.Await("WIDGET count=", widgetBudget); err != nil {
		t.Fatalf("the widget wrote nothing:\n%s", smoke.Readable(smoke.LastLines(screen.Text(), 8)))
	}
	drawn := strings.TrimPrefix(screen.Text(), before)
	want := "WIDGET count=5 newest=echo BBB keys=5 4 3 2 1"
	if !strings.Contains(drawn, want) {
		t.Fatalf("the widget's reading is not %q:\n%s", want,
			smoke.Readable(smoke.LastLines(screen.Text(), 8)))
	}
}

// widgetSession puts an interactive zsh on a pseudo-terminal with a two-row
// prompt and a scratch home.
//
// **Two rows and not one.** A prompt whose upper row is rewritten on every
// keystroke draws a ladder of prompts down the screen and a one-row `PS1`
// cannot see it (#2467) — and this test presses a key, which is exactly the
// event that redraws.
func widgetSession(t *testing.T) (*os.File, *smoke.Screen) {
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
	writeHomeFile(t, home, ".zshrc", "PS1=$'HWROW\\n"+widgetMark+"'\n")

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
	go func() { done <- driver.MainArgs(sh, []string{"zsh", "-i"}) }()
	t.Cleanup(func() {
		// The shell first, so nothing is left reading a terminal this test is
		// about to close, and then both ends of it.
		_, _ = control.WriteString("exit\n")
		select {
		case <-done:
		case <-time.After(widgetBudget):
			t.Error("the shell did not exit")
		}
		_ = terminal.Close()
		_ = control.Close()
	})
	if err := screen.Await(widgetMark, widgetBudget); err != nil {
		t.Fatalf("no first prompt: %v", err)
	}
	return control, screen
}

// widgetType sends a line and waits for the prompt that follows it.
//
// The wait is on the *next* prompt rather than on anything the line produced,
// which is internal/smoke's synchronization: the shell draws a prompt after it
// has taken the terminal back, and the two cannot be reordered. The screen is
// never cleared between waits — Seek moves a cursor — so a failure prints
// everything that was drawn.
func widgetType(t *testing.T, control *os.File, screen *smoke.Screen, line string) {
	t.Helper()
	if _, err := control.WriteString(line + "\n"); err != nil {
		t.Fatalf("typing %q: %v", line, err)
	}
	if err := screen.Await(widgetMark, widgetBudget); err != nil {
		t.Fatalf("no prompt after %q: %v", line, err)
	}
}

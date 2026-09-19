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
	"github.com/blairham/sh/internal/blocks"
	"github.com/blairham/sh/internal/pty"
	"github.com/blairham/sh/internal/smoke"
)

// The jump on a real terminal, with a prompt of two rows.
//
// jump_test.go drives the builtin and proves the ranking reaches `cd`. It
// cannot see the thing this repository has been caught by twice: a feature
// that computes the right answer and puts nothing on the screen. So this one
// asks the only question that matters to a person — after typing `z work`,
// does the prompt that comes back say they are somewhere else — and it asks it
// through a pseudo-terminal, of the front end, with the shell composed the way
// the binary composes it.
//
// **Two rows and not one.** A prompt whose upper row is rewritten on every
// keystroke draws a ladder of prompts down the screen and a one-row `PS1`
// cannot see it (#2467); the directory is put on the upper row here for
// exactly that reason, so this test is reading the row that a redraw is most
// likely to get wrong.

// promptMark is the last row of the prompt this session synchronizes on, and
// it is not text anybody types: a wait on a mark the typed line contains
// passes for a shell that echoed the line and ran nothing.
const promptMark = "sh> "

// ptyBudget turns a hang into a failure. driver/terminal_test.go measured the
// shape of the wait this is bounding.
const ptyBudget = 10 * time.Second

// jumpSession puts an interactive shell on a pseudo-terminal, with a store,
// a two-row prompt whose upper row is the working directory, and a scratch
// home.
func jumpSession(t *testing.T, home, store string) (*os.File, *smoke.Screen) {
	t.Helper()
	control, terminal, err := pty.Open()
	if errors.Is(err, pty.ErrUnsupported) {
		t.Skip("no pseudo-terminal on this platform")
	}
	if err != nil {
		t.Fatalf("opening a pseudo-terminal: %v", err)
	}
	if err := pty.SetSize(terminal, 24, 80); err != nil {
		t.Fatalf("sizing the terminal: %v", err)
	}
	rc := filepath.Join(home, "rc.sh")
	// The upper row is `$PWD` in single quotes, so it is expanded when the
	// prompt is drawn rather than when the file is read — which is what makes
	// the prompt itself the evidence that the shell moved.
	if err := os.WriteFile(rc, []byte("PS1='["+"$PWD"+"]\n"+promptMark+"'\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	sh, err := pickDialect("bash")
	if err != nil {
		t.Fatal(err)
	}
	sh.Name = "sh"
	sh = withDirJump(sh)
	sh.Stdin, sh.Stdout, sh.Stderr = terminal, terminal, terminal
	sh.Dir = home
	sh.Env = []string{
		"HOME=" + home,
		"ENV=" + rc,
		"PATH=/usr/bin:/bin",
		"TERM=dumb",
		"HISTFILE=" + filepath.Join(home, "hist"),
		blocks.DirVar + "=" + store,
	}
	screen := smoke.Watch(control)
	done := make(chan int, 1)
	go func() { done <- driver.MainArgs(sh, []string{"sh", "-i"}) }()
	t.Cleanup(func() {
		// The shell first, so nothing is left reading a terminal this test is
		// about to close, and then both ends of it.
		_, _ = control.WriteString("exit\n")
		select {
		case <-done:
		case <-time.After(ptyBudget):
			t.Error("the shell did not exit")
		}
		_ = terminal.Close()
		_ = control.Close()
	})
	if err := screen.Await(promptMark, ptyBudget); err != nil {
		t.Fatalf("no first prompt: %v", err)
	}
	return control, screen
}

// typeLine sends a line and waits for the prompt that follows it.
//
// The wait is on the *next* prompt rather than on anything the line produced,
// which is the synchronization internal/smoke arrived at: the shell draws a
// prompt after it has taken the terminal back, and the two cannot be
// reordered. The screen is never cleared between waits — Seek moves a cursor
// — so a failure prints everything that was drawn.
func typeLine(t *testing.T, control *os.File, screen *smoke.Screen, line string) {
	t.Helper()
	if _, err := control.WriteString(line + "\n"); err != nil {
		t.Fatalf("typing %q: %v", line, err)
	}
	if err := screen.Await(promptMark, ptyBudget); err != nil {
		t.Fatalf("no prompt after %q: %v", line, err)
	}
}

// A person types `z` and a word, and the prompt that comes back says they are
// somewhere else.
func TestTheJumpIsDrawnOnATerminal(t *testing.T) {
	root := t.TempDir()
	home := jumpTree(t, root, "home")
	work := jumpTree(t, root, "shellwork")
	store, _ := seedStore(t,
		blocks.Record{Command: "ls", Cwd: work, Start: minutesAgo(20)},
		blocks.Record{Command: "ls", Cwd: work, Start: minutesAgo(10)},
	)
	control, screen := jumpSession(t, home, store)
	before := screen.Text()
	if strings.Contains(before, "["+work+"]") {
		t.Fatalf("the session started in %q, so this cannot tell a jump from standing still", work)
	}
	typeLine(t, control, screen, "z shellwork")
	after := strings.TrimPrefix(screen.Text(), before)
	if !strings.Contains(after, "["+work+"]") {
		t.Fatalf("the prompt drawn after the jump does not say %q:\n%s",
			work, smoke.Readable(smoke.LastLines(screen.Text(), 6)))
	}
}

// And a jump that cannot be made says so on the terminal rather than moving
// the shell somewhere plausible.
func TestARefusedJumpLeavesTheShellWhereItWas(t *testing.T) {
	root := t.TempDir()
	home := jumpTree(t, root, "home")
	work := jumpTree(t, root, "shellwork")
	store, _ := seedStore(t, blocks.Record{Command: "ls", Cwd: work, Start: minutesAgo(10)})
	control, screen := jumpSession(t, home, store)
	typeLine(t, control, screen, "z nothinglikethis")
	drawn := screen.Text()
	if !strings.Contains(drawn, "no ranked directory matches") {
		t.Fatalf("no refusal on the terminal:\n%s", smoke.Readable(smoke.LastLines(drawn, 8)))
	}
	if !strings.Contains(drawn[strings.LastIndex(drawn, "no ranked directory matches"):], "["+home+"]") {
		t.Fatalf("the prompt after the refusal is not still %q:\n%s",
			home, smoke.Readable(smoke.LastLines(drawn, 8)))
	}
}

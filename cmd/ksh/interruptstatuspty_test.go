// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/pty"
	"github.com/blairham/sh/internal/smoke"
)

// **A ^C at the prompt leaves 258 in `$?`** (#5936), the status ksh93 gives a
// command SIGINT ended, where bash, zsh and dash leave 130.
//
// Measured 2026-10-04 against `/bin/ksh` — ksh93u+ — through a
// pseudo-terminal, `-f -i`: `true`, `abc`, ^C, then `$?` on the next line
// reads 258, with and without `set -o emacs`. This shell left 130. The
// control is the trap row, where ksh93 runs the handler and leaves the status
// alone — `0` after `true` — so the 258 is the interrupt's and not a status
// this shell writes for every ^C.
func TestControlCAtThePromptLeavesKshsSignalStatus(t *testing.T) {
	for _, tc := range []struct{ name, setup, want string }{
		{"with no trap", "", "258"},
		{"under a trap, the status stands", "trap 'ti=$?' INT", "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			control, screen := kshPromptSession(t)
			if tc.setup != "" {
				kshAnswer(t, control, screen, tc.setup, nil)
			}
			kshAnswer(t, control, screen, "true", nil)
			if _, err := control.WriteString("abc\x03"); err != nil {
				t.Fatalf("typing abc and ^C: %v", err)
			}
			awaitKshPrompt(t, screen)
			got := kshAnswer(t, control, screen, `print -r -- "st-$?-"`, regexp.MustCompile(`st-([0-9]+)-\r?\n`))
			if got != tc.want {
				t.Errorf("after ^C the next line read %q, want %q", got, tc.want)
			}
		})
	}
}

// kshRow and kshMark are the two rows of the prompt the session waits on; the
// upper row is drawn once per prompt, where the editor redraws the lower one
// on every key.
const (
	kshRow    = "KJROW"
	kshMark   = "kj> "
	kshBudget = 20 * time.Second
)

// kshPromptSession puts an interactive ksh on a pseudo-terminal with a
// two-row prompt and a scratch home.
func kshPromptSession(t *testing.T) (*os.File, *smoke.Screen) {
	t.Helper()
	home := t.TempDir()
	c, terminal, err := pty.Open()
	if errors.Is(err, pty.ErrUnsupported) {
		t.Skip("no pseudo-terminal on this platform")
	}
	if err != nil {
		t.Fatalf("opening a pseudo-terminal: %v", err)
	}
	if err := pty.SetSize(terminal, 24, 100); err != nil {
		t.Fatalf("sizing the terminal: %v", err)
	}
	sh := shell()
	sh.SystemStartupDirectory = t.TempDir()
	sh.Stdin, sh.Stdout, sh.Stderr = terminal, terminal, terminal
	sh.Dir = home
	sh.Env = []string{
		"HOME=" + home,
		"PATH=/usr/bin:/bin",
		"TERM=dumb",
		"HISTFILE=" + filepath.Join(home, "hist"),
		"PS1=" + kshRow + "\n" + kshMark,
	}
	screen := smoke.Watch(c)
	done := make(chan int, 1)
	go func() { done <- driver.MainArgs(sh, []string{"ksh", "-i"}) }()
	t.Cleanup(func() {
		_, _ = c.WriteString("exit\n")
		select {
		case <-done:
		case <-time.After(kshBudget):
			t.Error("the shell did not exit")
		}
		_ = terminal.Close()
		_ = c.Close()
	})
	awaitKshPrompt(t, screen)
	return c, screen
}

// awaitKshPrompt waits for both rows of a fresh prompt.
func awaitKshPrompt(t *testing.T, screen *smoke.Screen) {
	t.Helper()
	for _, mark := range []string{kshRow, kshMark} {
		if err := screen.Await(mark, kshBudget); err != nil {
			t.Fatalf("no prompt: %v", err)
		}
	}
}

// kshAnswer types line, answers what rx captured from its output, or nothing
// for a nil rx, and waits for the prompt after it.
func kshAnswer(t *testing.T, control *os.File, screen *smoke.Screen, line string, rx *regexp.Regexp) string {
	t.Helper()
	from := len(screen.Text())
	if _, err := control.WriteString(line + "\n"); err != nil {
		t.Fatalf("typing %q: %v", line, err)
	}
	got := ""
	if rx != nil {
		deadline := time.Now().Add(kshBudget)
		for {
			if m := rx.FindStringSubmatch(screen.Text()[from:]); m != nil {
				got = m[1]
				break
			}
			if !time.Now().Before(deadline) {
				t.Fatalf("waited for %s; drawn since:\n%s", rx, smoke.Readable(smoke.LastLines(screen.Text()[from:], 10)))
			}
			time.Sleep(2 * time.Millisecond)
		}
	}
	awaitKshPrompt(t, screen)
	return got
}

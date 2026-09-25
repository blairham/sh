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

// `setopt longlistjobs` puts the job's pid in the notice a finished background
// job gets — **on a terminal**, which is the only place that notice exists.
//
// The option is what decides it, which is why both states are typed into one
// session: with it unset this shell already wrote the short form, so a test
// that only asked the set half would pass for a shell that named the pid
// always. Until #4491 the name was accepted, remembered and acted on by
// nothing, and the two halves below were byte-identical.
//
// Measured 2026-09-25 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — on a pseudo-terminal with `TERM=dumb`, `PS1`
// and `PS2` exported empty and the shell started `-fiV +Z`:
//
//	setopt longlistjobs / : &   →  [1] 71984
//	                               [1]  + 71984 done       :
//	unsetopt longlistjobs / : & →  [1] 71990
//	                               [1]  + done       :
//
// This is zsh's own `W02jobs.ztst` asking the same question through `zpty`:
// its pattern for the second chunk wants one or more digits between the `+`
// and `done`.
//
// The assertion is on the **raw** bytes the terminal was given. A view with
// the escapes taken out is a view that could have normalized away the thing
// under test, and this repository has been burned by exactly that.
func TestLongListJobsNamesThePIDInACompletionNotice(t *testing.T) {
	control, screen := jobNoticeSession(t)

	// The option first, so the very next job's notice is the one it decides.
	jobNoticeType(t, control, screen, "setopt longlistjobs")
	before := screen.Text()
	jobNoticeType(t, control, screen, ": &")
	// The notice arrives before a prompt rather than the moment the job ends,
	// so the prompt jobNoticeType waited for is the marker that it has been
	// written — no sleep, and nothing is cleared between waits.
	long := screen.Text()[len(before):]

	jobNoticeType(t, control, screen, "unsetopt longlistjobs")
	before = screen.Text()
	jobNoticeType(t, control, screen, ": &")
	short := screen.Text()[len(before):]

	// `[1]  + <digits> done`, the row `jobs -l` writes. The pid is not spelled
	// out: this shell answers a job with no process of its own with an
	// invented identity, which is the same number its `[n] …` announcement
	// already carries, and what the option decides is whether the column is
	// there at all.
	named := regexp.MustCompile(`\[1\]  \+ [0-9]+ done`)
	bare := regexp.MustCompile(`\[1\]  \+ done`)
	if !named.MatchString(long) {
		t.Errorf("with the option set the notice is\n%s\nwant a row matching %s",
			smoke.Readable(smoke.LastLines(long, 8)), named)
	}
	if !bare.MatchString(short) {
		t.Errorf("with the option unset the notice is\n%s\nwant a row matching %s",
			smoke.Readable(smoke.LastLines(short, 8)), bare)
	}
	if named.MatchString(short) {
		t.Errorf("with the option unset the notice still names a pid:\n%s",
			smoke.Readable(smoke.LastLines(short, 8)))
	}
}

// jobNoticeMark is the last row of the prompt this session synchronizes on,
// and it is not text anybody types: a wait on a mark the typed line contains
// passes for a shell that echoed the line and ran nothing.
const jobNoticeMark = "jn> "

// jobNoticeBudget turns a hang into a failure.
const jobNoticeBudget = 20 * time.Second

// jobNoticeSession puts an interactive zsh on a pseudo-terminal with a two-row
// prompt and a scratch home.
//
// **Two rows and not one.** A prompt whose upper row is rewritten on every
// keystroke draws a ladder of prompts down the screen and a one-row `PS1`
// cannot see it (#2467).
func jobNoticeSession(t *testing.T) (*os.File, *smoke.Screen) {
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
	writeHomeFile(t, home, ".zshrc", "PS1=$'JNROW\\n"+jobNoticeMark+"'\n")

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
		// about to close, and then both ends of it. Twice, because a shell
		// that still holds a job refuses the first one.
		_, _ = control.WriteString("exit\nexit\n")
		select {
		case <-done:
		case <-time.After(jobNoticeBudget):
			t.Error("the shell did not exit")
		}
		_ = terminal.Close()
		_ = control.Close()
	})
	if err := screen.Await(jobNoticeMark, jobNoticeBudget); err != nil {
		t.Fatalf("no first prompt: %v", err)
	}
	return control, screen
}

// jobNoticeType sends a line and waits for the prompt that follows it.
//
// The wait is on the *next* prompt rather than on anything the line produced,
// which is internal/smoke's synchronization: the shell draws a prompt after it
// has taken the terminal back, and the two cannot be reordered. The screen is
// never cleared between waits — Seek moves a cursor — so a failure prints
// everything that was drawn.
func jobNoticeType(t *testing.T, control *os.File, screen *smoke.Screen, line string) {
	t.Helper()
	if _, err := control.WriteString(line + "\n"); err != nil {
		t.Fatalf("typing %q: %v", line, err)
	}
	if err := screen.Await(jobNoticeMark, jobNoticeBudget); err != nil {
		t.Fatalf("no prompt after %q: %v", line, err)
	}
}

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

// An interactive zsh that is leaving sends SIGHUP to the jobs it is
// abandoning and says how many — `setopt hup`, which is on in a fresh shell.
//
// The sentence exists only where a session does, which is why this is a
// pseudo-terminal and not a `-c`. Until #4509 `hup` was one of the recorded
// names: accepted, remembered, reported by `[[ -o hup ]]`, and read by
// nothing, so both states of it left the job running and said nothing.
//
// Measured 2026-09-25 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — on a pseudo-terminal with `TERM=dumb`, `PS1`
// and `PS2` exported empty and the shell started `-fiV +Z`, which is
// interactive and **not** a login shell:
//
//	setopt no_check_jobs / sleep 5 & / exit  →  zsh: warning: 1 jobs SIGHUPed
//	setopt no_check_jobs no_hup / same       →  nothing, and the job runs on
//
// This is zsh's own `W02jobs.ztst` asking the same question through `zpty`:
// six of that file's chunks want a `zsh:*SIGHUPed*` line after the session
// ends.
//
// Both states are driven, in two sessions, because the option is what decides
// it and one session can only leave once: a test that asked the set half
// alone would pass for a shell that said this always.
//
// The assertion is on the **raw** bytes the terminal was given. A view with
// the escapes taken out is a view that could have normalized away the thing
// under test, and this repository has been burned by exactly that.
func TestAnExitingSessionSaysHowManyJobsItHungUp(t *testing.T) {
	const want = "warning: 1 jobs SIGHUPed"
	on := hupAtExitSession(t, "setopt no_check_jobs")
	if !strings.Contains(on, want) {
		t.Errorf("with `hup` set the session ended with\n%s\nwant a line containing %q",
			smoke.Readable(smoke.LastLines(on, 10)), want)
	}
	// And on the near side of the EXIT trap, which is the order this shell
	// writes them in — the last occurrence, because the trap's own line was
	// echoed back when it was typed.
	if strings.LastIndex(on, want) > strings.LastIndex(on, hupAtExitEnd) {
		t.Errorf("the sentence came after the EXIT trap's word:\n%s",
			smoke.Readable(smoke.LastLines(on, 10)))
	}
	off := hupAtExitSession(t, "setopt no_check_jobs no_hup")
	if strings.Contains(off, "SIGHUPed") {
		t.Errorf("with `unsetopt hup` the session still said it hung the job up:\n%s",
			smoke.Readable(smoke.LastLines(off, 10)))
	}
	// And the negative row is a real session rather than a shell that fell
	// over before it got there: the job's own announcement has to be on the
	// screen either way, or the two halves are not the same measurement.
	for _, s := range []struct{ why, text string }{{"set", on}, {"unset", off}} {
		if !strings.Contains(s.text, "[1]") {
			t.Errorf("with the option %s the job was never announced:\n%s",
				s.why, smoke.Readable(smoke.LastLines(s.text, 10)))
		}
	}
}

// hupAtExitMark is the last row of this session's prompt, and is not text
// anybody types: a wait on a mark the typed line contains passes for a shell
// that echoed the line and ran nothing.
const hupAtExitMark = "hx> "

const hupAtExitBudget = 20 * time.Second

// hupAtExitSession runs one session to its end and hands back everything the
// terminal was given.
//
// One session per row, because the sentence is written as the shell leaves
// and a shell leaves once. The `sleep` outlives the session on purpose — that
// is the job being abandoned — and it is signaled by the shell under test in
// one row and left running in the other, so it is killed here rather than
// waited for.
func hupAtExitSession(t *testing.T, setup string) string {
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
	// Two rows and not one, for the reason jobNoticeSession gives: a prompt
	// whose upper row is rewritten at every keystroke draws a ladder of
	// prompts and a one-row PS1 cannot see it (#2467).
	writeHomeFile(t, home, ".zshrc", "PS1=$'HXROW\\n"+hupAtExitMark+"'\n")

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
		_ = terminal.Close()
		_ = control.Close()
	})
	if err := screen.Await(hupAtExitMark, hupAtExitBudget); err != nil {
		t.Fatalf("no first prompt: %v", err)
	}
	hupAtExitType(t, control, screen, setup)
	// The EXIT trap is this session's **marker** and not its subject: the
	// hangup falls on the near side of it in this shell — measured, and
	// Semantics.HangupAtExitPrecedesTheExitTrap — so a screen holding the
	// trap's word is a screen the sentence has already reached if it was
	// ever coming. Waiting on the shell's *process* instead is a race, and a
	// flaky one: the run returns when the last write is issued and not when
	// the terminal has been read, so one run in six read the screen a
	// moment early and reported the line missing.
	hupAtExitType(t, control, screen, "trap 'print "+hupAtExitEnd+"' EXIT")
	hupAtExitType(t, control, screen, "sleep 5 &")

	// And then the ending itself, which is what this session is for. Twice,
	// because `checkjobs` holds the first one in the row that leaves it on —
	// the same two lines the suite's own helper sends for the same reason.
	if _, err := control.WriteString("exit\nexit\n"); err != nil {
		t.Fatalf("typing the exit: %v", err)
	}
	if err := screen.Await(hupAtExitEnd, hupAtExitBudget); err != nil {
		t.Fatalf("the session never reached its EXIT trap: %v", err)
	}
	select {
	case <-done:
	case <-time.After(hupAtExitBudget):
		t.Fatalf("the shell did not exit; the screen was\n%s", smoke.Readable(screen.Text()))
	}
	// The job is the session's to abandon, not this test's to wait out.
	return screen.Text()
}

// hupAtExitEnd is written by the session's own EXIT trap, and is the marker
// both rows read the screen against. Not a word either row types as input:
// a wait on text the typed line contains passes for a shell that echoed the
// line and ran nothing.
const hupAtExitEnd = "HXLEFT"

// hupAtExitType sends a line and waits for the prompt that follows it — the
// same synchronization internal/smoke uses everywhere, and no sleep. The
// screen is never cleared between waits, so a failure prints everything that
// was drawn.
func hupAtExitType(t *testing.T, control *os.File, screen *smoke.Screen, line string) {
	t.Helper()
	if _, err := control.WriteString(line + "\n"); err != nil {
		t.Fatalf("typing %q: %v", line, err)
	}
	if err := screen.Await(hupAtExitMark, hupAtExitBudget); err != nil {
		t.Fatalf("no prompt after %q: %v", line, err)
	}
}

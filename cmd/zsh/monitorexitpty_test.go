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

// A **script** whose monitor is on accounts for the jobs it leaves running:
// it names them and then says it hung them up, with no prompt anywhere.
//
// This is the surface #4542 is about, and it exists only here. The sentences
// are written as a shell ends, so an `interp` test builds the state by hand
// and this one has to be a real invocation; and the monitor is granted only
// where there is a terminal, so it has to be a real invocation **on a
// pseudo-terminal**. `-c` and a plain script file both leave the monitor off
// and neither can reach any of this.
//
// Measured 2026-09-25 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — on a pseudo-terminal with `TERM=dumb`, the
// shell given a script file and started `-fm`, which is **not** interactive:
//
//	sleep 3 &                     [1] <pid>
//	                              <script>:3: you have running jobs.
//	                              <script>:3: warning: 1 jobs SIGHUPed
//
// and with `-f` in place of `-fm`, the same script writes neither line.
//
// **The monitor is the noun and interactivity is not.** They agree in every
// shell a person sits at, because a session turns the monitor on, so the two
// are told apart only by a row that moves one and holds the other: this file
// holds interactivity fixed at *no* and moves the monitor, and
// hupatexitpty_test.go holds it fixed at *yes* and reaches the same two
// sentences. Until #4542 the hangup read Runner.Interactive and the sentence
// naming the jobs was never written outside a prompt at all, so both rows
// below were silent.
//
// The line number the reference writes is **not** asserted, and that is
// deliberate rather than an oversight: this shell writes the script's path
// and no line, which is #4545, measured and left rather than guessed at. What
// is asserted is the wording, which is this shell's own and is right on both
// routes.

const monitorExitBudget = 20 * time.Second

// monitorExitScript runs one script to its end under a pseudo-terminal and
// hands back everything the terminal was given.
//
// The `sleep` outlives the shell on purpose — that is the job being abandoned
// — so it is killed here rather than waited for, and it is long enough that
// it cannot end of its own accord between the shell starting and the shell
// leaving.
func monitorExitScript(t *testing.T, args []string, body string) string {
	t.Helper()
	home := scratchHome(t)
	path := filepath.Join(home, "job.zsh")
	if err := os.WriteFile(path, []byte(monitorExitTrap+body), 0o600); err != nil {
		t.Fatalf("writing the script: %v", err)
	}
	return monitorExitRun(t, home, append(append([]string{"zsh"}, args...), path), monitorExitEnd)
}

// monitorExitCommandString is monitorExitScript on the `-c` route: the same
// terminal, the same EXIT-trap marker in front of the body — so a line number
// the shell writes is one more than the body's own — and the body handed over
// as the command string.
//
// last is what the shell writes last, in order, and what the run waits for —
// not always the marker on this route: a string that runs off its end hangs
// its jobs up *after* the EXIT trap, and an `exit` that wrote the
// jobs-at-exit sentence runs no EXIT trap at all (see
// interp.Diagnostics.JobsAtExitOnACommandString). So each row names the
// text it ends on rather than a screen that has only reached the trap.
func monitorExitCommandString(t *testing.T, body string, last ...string) string {
	t.Helper()
	home := scratchHome(t)
	return monitorExitRun(t, home, []string{"zsh", "-c", monitorExitTrap + body}, last...)
}

// monitorExitRun is the run both of those share, so the wait discipline
// recorded below is in one place.
func monitorExitRun(t *testing.T, home string, argv []string, last ...string) string {
	t.Helper()
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
	select {
	case <-done:
	case <-time.After(monitorExitBudget):
		t.Fatalf("the script did not end; the screen was\n%s", smoke.Readable(screen.Text()))
	}
	// The shell's run returns when its last write is issued rather than when
	// the terminal has been read, so the screen is given a moment to catch
	// up — and the thing waited on has to be written *after* everything
	// asserted on, or the wait orders nothing. This waited on the script's
	// own last line until #4577, which is written before the shell has said
	// anything about its jobs at all: the fence proved the script had
	// finished and proved nothing about the output the assertions read, and
	// a required CI check duly failed with the first sentence on the screen
	// and the second not yet read.
	//
	// The EXIT trap is the marker instead, as it is in hupatexitpty_test.go,
	// and for the reason recorded there:
	// Semantics.HangupAtExitPrecedesTheExitTrap, so both sentences fall on
	// the near side of it. Re-measured 2026-09-26 against
	// `/opt/homebrew/bin/zsh` — zsh 5.9.2 (aarch64-apple-darwin25.4.0) — on a
	// pseudo-terminal, with a `trap` line added to each of the three scripts
	// below, the trap changes neither row and its line comes last every time:
	//
	//	-fm, backgrounded sleep   [1] <pid> / <fence> /
	//	                          <script>: you have running jobs. /
	//	                          <script>: warning: 1 jobs SIGHUPed / <end>
	//	-f, the same script       <fence> / <end>
	//	-fm, then `kill -STOP %1` [1] <pid> / <fence> /
	//	                          <script>: you have suspended jobs. / <end>
	//
	// That last property is what the monitor-off row needs and could not get
	// from any sentence: it asserts an **absence**, and an absence has
	// nothing of its own to wait for, so it wants a mark the shell writes
	// after the point where the sentence would have been.
	for _, text := range last {
		if err := screen.Await(text, monitorExitBudget); err != nil {
			t.Fatalf("the shell never finished writing: %v\n%s",
				err, smoke.Readable(screen.Text()))
		}
	}
	return screen.Text()
}

// monitorExitFence is written by the script's own last line, so a row that
// fell over early is a failure rather than a silent negative. Not text any of
// these scripts could echo back: nothing types at this shell.
//
// It is a check and no longer a fence, whatever its name still says: the two
// are different questions — "did the script get to the end" against "has the
// shell finished writing" — and #4577 is what asking the first one twice
// cost.
const monitorExitFence = "MX-FENCE-4542"

// monitorExitEnd is the last thing written on any of these runs, because the
// EXIT trap that writes it is the last thing the shell runs that can reach
// the terminal. It is what every row waits on.
const monitorExitEnd = "MX-END-4577"

// monitorExitTrap goes on the front of every script here, so that no row can
// be written that waits on the wrong line.
const monitorExitTrap = "trap 'print -r -- " + monitorExitEnd + "' EXIT\n"

// The two sentences, with the monitor on and with it off, in one test because
// one row alone is not a measurement: a shell that said this always would
// pass the first and a shell that never said it would pass the second.
func TestAScriptRunningTheMonitorAccountsForTheJobsItLeaves(t *testing.T) {
	const body = "/bin/sleep 30 &\nprint -r -- " + monitorExitFence + "\n"
	on := monitorExitScript(t, []string{"-fm"}, body)
	for _, want := range []string{"you have running jobs.", "warning: 1 jobs SIGHUPed"} {
		if !strings.Contains(on, want) {
			t.Errorf("with the monitor on the script ended with\n%s\nwant a line containing %q",
				smoke.Readable(smoke.LastLines(on, 10)), want)
		}
	}
	// And in that order, which is the reference's: the jobs are named and
	// then the hangup is reported.
	if i, j := strings.Index(on, "running jobs."), strings.Index(on, "SIGHUPed"); i < 0 || j < 0 || i > j {
		t.Errorf("the two sentences came out in the wrong order:\n%s",
			smoke.Readable(smoke.LastLines(on, 10)))
	}
	off := monitorExitScript(t, []string{"-f"}, body)
	if strings.Contains(off, "running jobs.") || strings.Contains(off, "SIGHUPed") {
		t.Errorf("with the monitor off the script still accounted for its job:\n%s",
			smoke.Readable(smoke.LastLines(off, 10)))
	}
	// And the negative row is a real run rather than a shell that fell over
	// before it got there: the fence has to be on the screen either way, or
	// the two halves are not the same measurement. The job's announcement is
	// deliberately *not* used for that — it rides on the monitor too, so it
	// is absent in the second row for the very reason under test.
	for _, s := range []struct{ why, text string }{{"on", on}, {"off", off}} {
		if !strings.Contains(s.text, monitorExitFence) {
			t.Errorf("with the monitor %s the script never reached its last line:\n%s",
				s.why, smoke.Readable(smoke.LastLines(s.text, 10)))
		}
	}
}

// A job stopped by the script's **last** command is known to have stopped by
// the time the shell chooses between the two sentences.
//
// A session sweeps the stop notices between one command and the next, so at a
// prompt every job's state is current by the time anything asks. A script's
// last command is followed by no command at all, and this reached the end
// with the job still reading as running: it wrote the running-jobs sentence
// and then hung up a job the reference leaves alone.
//
// Measured 2026-09-25 against zsh 5.9.2, `zsh -fm` over a script that
// backgrounds a `sleep` and stops it: `<script>:N: you have suspended jobs.`
// and **no** hangup line at all, because a stopped job is continued rather
// than hung up (Semantics.HangupAtExitSkipsStoppedJobs).
//
// Both halves are asserted. Either one alone passes for a shell that has the
// other wrong — a shell that never hangs anything up would pass the second,
// and one that always says `suspended` would pass the first.
//
// **One attempt, since #4558.** This used to run eight times and take the
// first that passed, because the sweep took only what the goroutine waiting on
// the job had already been told and `kill -STOP %1` returns when the signal is
// *sent* — so where that `kill` is the script's last command, the shell could
// reach its exit before that goroutine had been scheduled. It usually had, on
// an idle machine, and on a loaded CI runner it had not. The way out now waits
// for a stop it asked for and has not been told about, bounded, and only for
// the jobs it asked to stop; interp's TestAStopTheScriptAskedForIsWaitedForOnTheWayOut
// is the deterministic form of the same question.
func TestAJobStoppedByTheLastCommandIsNoticedBeforeTheScriptLeaves(t *testing.T) {
	got := monitorExitScript(t, []string{"-fm"},
		"/bin/sleep 30 &\nprint -r -- "+monitorExitFence+"\nkill -STOP %1\n")
	if !strings.Contains(got, "you have suspended jobs.") {
		t.Errorf("the script did not write the suspended-jobs sentence; it ended with\n%s",
			smoke.Readable(smoke.LastLines(got, 10)))
	}
	if strings.Contains(got, "SIGHUPed") {
		t.Errorf("the script hung up a job that was stopped:\n%s",
			smoke.Readable(smoke.LastLines(got, 10)))
	}
}

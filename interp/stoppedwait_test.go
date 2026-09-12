// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// A `&` job's process is waited for by the goroutine that started it, and
// before #2227 that wait was os/exec's — an ordinary waitpid, which a stopped
// child never returns from. So a `wait` for such a job blocked for as long as
// something outside the shell took to resume or kill it.
//
// These tests stand in for the front end's wait rather than for the kernel,
// which is the only way to say "and now it stops" on demand. The child is a
// real process all the same: a background job reaches this path only by
// starting one, and the stand-in reaps it, because a wait that says a command
// stopped has by definition not collected it.

// stoppedThenHeld is a WaitForCommand that reports one stop and then does not
// come back, which is what a process stopped by SIGSTOP really does. The
// channel is how the test lets the goroutine go at the end.
func stoppedThenHeld(t *testing.T) (func(int) (Wait, error), chan struct{}) {
	t.Helper()
	held := make(chan struct{})
	t.Cleanup(func() { close(held) })
	first := true
	return func(pid int) (Wait, error) {
		if first {
			first = false
			reapForTest(t, pid)
			return Wait{Signal: syscall.SIGSTOP, Stopped: true}, nil
		}
		<-held
		return Wait{}, nil
	}, held
}

// reapForTest collects the child this stand-in is about to lie about, because
// nothing else will: the shell believes the wait reaped it.
func reapForTest(t *testing.T, pid int) {
	t.Helper()
	for {
		var ws syscall.WaitStatus
		if _, err := syscall.Wait4(pid, &ws, 0, nil); err != syscall.EINTR {
			return
		}
	}
}

func stoppedWaitRunner(t *testing.T, axis Answer, monitor bool, wait func(int) (Wait, error)) (*Runner, *syntax.File, func() string) {
	t.Helper()
	file, err := syntax.Parse(echoCmd+" &\nwait $!\n", syntax.Core())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out := sink(t)
	sem := permissive()
	sem.SignalDeathStatusIsTwoFiftySix = No
	sem.AnnouncesBackgroundJob = No
	sem.WaitGivesUpOnAStoppedJob = axis
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{Stdout: out, Stderr: out, Semantics: &sem, Diagnostics: &dg, Name: "testsh"})
	if monitor {
		if code := r.SetOptionLetters("m", true); code != 0 {
			t.Fatalf("set -m: status %d", code)
		}
	}
	r.WaitForCommand = wait
	return r, file, func() string { return read(t, out) }
}

// TestWaitGivesUpOnAStoppedJob is the dialect that does not go on waiting: the
// status is a command killed by the stop signal, which is what bash answers.
func TestWaitGivesUpOnAStoppedJob(t *testing.T) {
	wait, _ := stoppedThenHeld(t)
	r, file, _ := stoppedWaitRunner(t, Yes, true, wait)
	if _, err := r.Run(context.Background(), file); err != nil {
		t.Fatal(err)
	}
	if got, want := r.ExitStatus(), 128+int(syscall.SIGSTOP); got != want {
		t.Errorf("status %d, want %d — 128 plus the stop signal", got, want)
	}
}

// TestWaitWaitsOutAStoppedJob is the other answer, and the one that must not
// be reached by simply never noticing: the wait goes past the stop and reports
// what the job did when it finally ended.
func TestWaitWaitsOutAStoppedJob(t *testing.T) {
	first := true
	wait := func(pid int) (Wait, error) {
		if first {
			first = false
			reapForTest(t, pid)
			return Wait{Signal: syscall.SIGSTOP, Stopped: true}, nil
		}
		return Wait{Status: 7}, nil
	}
	r, file, _ := stoppedWaitRunner(t, No, true, wait)
	if _, err := r.Run(context.Background(), file); err != nil {
		t.Fatal(err)
	}
	if got := r.ExitStatus(); got != 7 {
		t.Errorf("status %d, want 7 — the job's own, once it ended", got)
	}
}

// TestTheMonitorIsWhatAsksAboutAStoppedJob holds down the condition rather
// than the answer. With the monitor off a `&` job is not in a group of its
// own and the whole panel waits for it whatever it does, so the front end's
// wait is not reached and the axis is not asked: a shell that asked it there
// would refuse a script that had chosen no dialect for a question the panel
// does not have.
func TestTheMonitorIsWhatAsksAboutAStoppedJob(t *testing.T) {
	asked := 0
	wait := func(pid int) (Wait, error) {
		asked++
		reapForTest(t, pid)
		return Wait{Signal: syscall.SIGSTOP, Stopped: true}, nil
	}
	r, file, read := stoppedWaitRunner(t, Unspecified, false, wait)
	if _, err := r.Run(context.Background(), file); err != nil {
		t.Fatal(err)
	}
	if asked != 0 {
		t.Errorf("the front end's wait was used %d times, want 0 with the monitor off", asked)
	}
	if got := r.ExitStatus(); got != 0 {
		t.Errorf("status %d, want 0 — the job's own", got)
	}
	if said := read(); strings.Contains(said, "no dialect was chosen") {
		t.Errorf("said %q, want no refusal: the axis is not this shell's question", said)
	}
}

// TestAStoppedJobIsRefusedWhenUnanswered is the same axis where it *is* the
// question, so an unanswered one is refused rather than guessed.
func TestAStoppedJobIsRefusedWhenUnanswered(t *testing.T) {
	wait, _ := stoppedThenHeld(t)
	r, file, read := stoppedWaitRunner(t, Unspecified, true, wait)
	if _, err := r.Run(context.Background(), file); err != nil {
		t.Fatal(err)
	}
	if said := read(); !strings.Contains(said, "no dialect was chosen") {
		t.Errorf("said %q, want a refusal naming the axis", said)
	}
}

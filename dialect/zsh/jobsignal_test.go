// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"syscall"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// This shell names the signal that ended a job, in its own words for it.
//
// Measured 2026-09-25 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — on a pseudo-terminal with `TERM=dumb` and the
// shell started `-fiV +Z`, one `sleep 30 &` and one `kill -NAME %%` per row.
// The notice is measured where one exists, in cmd/zsh; what is asked here is
// the table and the row it is rendered into.
//
// The words are this shell's and not the machine's, which is why there is a
// table at all: the host's C library says `Abort trap`, `Bad system call`,
// `Alarm clock` and `Illegal instruction` where this shell says `abort`,
// `invalid system call`, `alarm` and `illegal hardware instruction`, and it
// writes the signal's number after every one of them where this shell writes
// none.
func TestTheSignalThatEndedAJobIsNamed(t *testing.T) {
	dg := zsh.Diagnostics()
	if got, want := dg.JobSignaled, "%[1]s"; got != want {
		t.Errorf("JobSignaled = %q, want %q — the word alone, with no number after it", got, want)
	}
	for _, c := range []struct {
		sig  syscall.Signal
		want string
	}{
		{syscall.SIGTERM, "terminated"},
		{syscall.SIGHUP, "hangup"},
		{syscall.SIGINT, "interrupt"},
		{syscall.SIGKILL, "killed"},
		{syscall.SIGQUIT, "quit"},
		{syscall.SIGPIPE, "broken pipe"},
		{syscall.SIGABRT, "abort"},
		{syscall.SIGSEGV, "segmentation fault"},
		{syscall.SIGALRM, "alarm"},
		{syscall.SIGILL, "illegal hardware instruction"},
		{syscall.SIGSYS, "invalid system call"},
		{syscall.SIGUSR1, "user-defined signal 1"},
		{syscall.SIGXCPU, "cpu limit exceeded"},
	} {
		if got := dg.SignalDescriptions[c.sig]; got != c.want {
			t.Errorf("signal %d is %q, want %q", c.sig, got, c.want)
		}
	}
}

// The state column is nine wide with two spaces after it, and not a flat
// eleven.
//
// The two are the same for every word that fits in nine and part for one that
// does not, which is why the signal words are what found it: measured on
// 5.9.2, `hangup` is followed by five spaces and `interrupt` by two — eleven
// either way, and what a flat eleven predicts — while `terminated` is followed
// by **two**, for twelve, and `segmentation fault` by two rather than by
// nothing at all.
//
// The `done`/`running` half is here beside it because that is the half that
// must not move: every word this shell wrote before the signal ones arrived is
// nine or fewer, so the change is byte-for-byte invisible to them, and a test
// that only checked the new words could not say so.
func TestTheStateColumnIsNineWideWithTwoSpacesAfterIt(t *testing.T) {
	dg := zsh.Diagnostics()
	for _, c := range []struct{ state, want string }{
		// Longer than the column: two spaces, never one, and never none.
		{"terminated", "[1]  + terminated  sleep 30"},
		{"broken pipe", "[1]  + broken pipe  sleep 30"},
		{"segmentation fault", "[1]  + segmentation fault  sleep 30"},
		// At the column's width and inside it: unchanged from the flat
		// eleven this used to be.
		{"interrupt", "[1]  + interrupt  sleep 30"},
		{"hangup", "[1]  + hangup     sleep 30"},
		{"killed", "[1]  + killed     sleep 30"},
		{"done", "[1]  + done       sleep 30"},
		{"running", "[1]  + running    sleep 30"},
		{"suspended", "[1]  + suspended  sleep 30"},
	} {
		got := interp.Wording(dg.JobLine, "", 1, "+", c.state, "sleep 30")
		if got != c.want {
			t.Errorf("row = %q, want %q", got, c.want)
		}
	}
	// And `jobs -l`'s row, which carries the pid and the same column — the
	// shape `setopt longlistjobs` puts in a notice (#4491).
	const want = "[1]  + 97103 terminated  sleep 30"
	if got := interp.Wording(dg.JobLineLong, "", 1, "+", 97103, "terminated", "sleep 30"); got != want {
		t.Errorf("long row = %q, want %q", got, want)
	}
}

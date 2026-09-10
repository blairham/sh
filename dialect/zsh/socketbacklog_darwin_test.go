// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
	"time"

	"github.com/blairham/sh/internal/dialecttest"
)

// **A listener holds one connection and refuses the next.**
//
// That is what zsh's own listener does *on this operating system*, and the
// file name is the whole of that qualification. Measured 2026-09-10 on both,
// against real zsh and against this shell, because the first version of this
// test assumed the behavior was the shell's when it is the kernel's:
//
//	macOS 26 / zsh 5.9.2   queues 1, and the next connection is refused
//	Debian 12 / zsh 5.9    queues 2, and the next connection **blocks**
//
// This shell answers the same as zsh on each — 1 then refused here, 2 then
// blocked there — from one `listen(fd, 1)`, so the implementation is right on
// both and it was the *assertion* that was platform-blind. It ran on Linux CI,
// filled the queue, connected once more, and took the package's whole ten
// minutes down with it.
//
// So this is not a weakened assertion moved behind a tag to make a build pass.
// The property zsocketBacklog buys — a queue of one or two rather than a
// runtime default's hundred and twenty-eight — has a visible consequence on
// exactly one of the two platforms, and it is asserted where it is visible.
//
// The deadline is a second, separate thing. A test that can hang is a defect
// whatever it asserts, so the run is given fifteen seconds: if a future
// backlog large enough to swallow the second connection ever *did* make this
// block, the answer is a failure naming the reason rather than a timeout
// naming the package.
func TestZsocketListenerHoldsOneConnectionAndRefusesTheNext(t *testing.T) {
	dir := socketDir(t)
	src := `zsocket -l sock
zsocket sock
print -r -- "first=$?"
zsocket sock 2>&1
print -r -- "second=$?"`
	// Parsed here rather than only inside the run, so the one path in the
	// helper that can end the test lives on the test's own goroutine.
	preset.Parse(t, src)

	type outcome struct {
		out    string
		status int
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		out, st, err := preset.Combined(t, dialecttest.Base{
			Dir: dir, Vars: map[string]string{"PATH": dir},
		}, src)
		done <- outcome{out, st, err}
	}()

	var got outcome
	select {
	case got = <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("a second connection to a full backlog neither succeeded nor was refused within 15s: " +
			"it blocked, which is what Linux does and this platform does not")
	}
	if got.err != nil {
		t.Fatalf("run: %v", got.err)
	}
	want := "first=0\n" +
		"zsh:zsocket:4: connection failed: connection refused\nsecond=1\n"
	if got.out != want || got.status != 0 {
		t.Errorf("the backlog = %q (status %d), want %q", got.out, got.status, want)
	}
}

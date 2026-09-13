// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
)

// A here-document body holding a continued line used to end at the first line
// that merely looked like the delimiter, and everything under it — the rest of
// somebody's *text* — was run as commands. Where that text reads standard
// input and standard input is a terminal, the script never returns.
//
// This is #2430's teeth and the reason it is filed as a hang rather than as a
// wrong body. Measured on the commit before the fix, with a pseudo-terminal on
// all three descriptors: bash runs the same script in 4.6 ms and this shell
// was still sitting there after 95 seconds, its own process and the child it
// should never have started both at 0.00 of CPU and both in an interruptible
// sleep. Not slow — blocked, on a read of a terminal nobody was typing at.
//
// The assertion is what the script *finishes* with, never that it hangs: a
// regression fails on the deadline in sessionBudget rather than stopping the
// suite. The terminal is what makes it the reported shape; a pipe left open
// blocks the same way, and neither is a thing to wait on without a bound.
func TestABodysOwnTextIsNotRunAsCommandsAtATerminal(t *testing.T) {
	_, tty := terminal(t)
	// Line 2 asks for line 3, so the first `EOF` is not the delimiter and the
	// `read` under it is body. Read as a command it would take the terminal,
	// which has nothing on it and never will.
	path := writeScript(t, "read line <<EOF\nnext\\\nEOF\nread blocked\nEOF\necho \"[$line]\"\n")

	sh := shell()
	sh.Stdin = tty
	var out, errs bytes.Buffer
	sh.Stdout, sh.Stderr = &out, &errs

	done := make(chan int, 1)
	go func() { done <- driver.MainArgs(sh, []string{"testsh", path}) }()

	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("status = %d, stderr %q", code, errs.String())
		}
		if got, want := out.String(), "[nextEOF]\n"; got != want {
			t.Errorf("output = %q, want %q — the body stopped at the wrong line", got, want)
		}
	case <-time.After(sessionBudget):
		// The goroutine is blocked on the terminal; the cleanup that closes
		// it is what lets it go.
		t.Fatalf("the script did not finish in %v: the body's own text was run as commands and one of them read the terminal", sessionBudget)
	}
}

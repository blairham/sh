// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"bytes"
	"context"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/blairham/sh/syntax"
)

// raisingBrokenPipe is a pipe whose reader has gone, and it plays the kernel's
// part as well as the pipe's: each write puts `copies` SIGPIPEs on the shell's
// channel, the way the runtime forwards the copies the kernel raised, and then
// fails after `wrote` bytes went through.
//
// The two shapes it stands for are measured (#7201). A write that finds the
// reader already gone fails at 0 bytes and raises one copy. A write the reader
// leaves part-way through goes partly through and raises two: one on the short
// write, one on the retry that finds no reader. The copies go on the channel
// from inside the write, so they are there before the shell records its credit,
// which is the order the real ones arrive in when the runtime is quick.
type raisingBrokenPipe struct {
	r      *Runner
	copies int
	wrote  int
}

func (p *raisingBrokenPipe) Write(b []byte) (int, error) {
	for range p.copies {
		p.r.sigs().ch <- syscall.SIGPIPE
	}
	return min(p.wrote, len(b)), &os.PathError{Op: "write", Path: "|1", Err: syscall.EPIPE}
}

// handledRuns runs a handled broken pipe against a stream that raises copies
// and fails after wrote bytes, and counts how often the handler ran.
func handledRuns(t *testing.T, copies, wrote int) (int, string) {
	t.Helper()
	sem := PosixSemantics()
	sem.BuiltinWriteErrorFailsTheCommand = Yes
	// Every arrival runs its handler, so a copy that got past the credits is
	// counted rather than folded into the turn the write's own signal has.
	sem.WaitingSignalArrivingAgainRunsOnce = No
	diag := Diagnostics{BuiltinWriteError: "%[1]s: write error: %[2]s"}
	var errOut bytes.Buffer
	r := newTestRunner(t, &Runner{Stderr: &errOut, Semantics: &sem, Diagnostics: &diag})
	r.Stdout = &raisingBrokenPipe{r: r, copies: copies, wrote: wrote}
	f, err := syntax.Parse(`trap 'echo handled >&2' PIPE; echo hi; echo after >&2`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return strings.Count(errOut.String(), "handled"), errOut.String()
}

// One failed write is one signal to the script, however many copies of it the
// kernel raised: the shell answers it at the write, and the runtime's copies
// are what it already answered.
func TestAWriteTheReaderLeftPartWayRunsTheHandlerOnce(t *testing.T) {
	for _, c := range []struct {
		name          string
		copies, wrote int
	}{
		{"the reader gone before the write", 1, 0},
		{"the reader gone part-way through", 2, 2},
	} {
		if n, errOut := handledRuns(t, c.copies, c.wrote); n != 1 {
			t.Errorf("%s: the handler ran %d times, want once (stderr %q)", c.name, n, errOut)
		}
	}
}

// The control: a copy beyond the ones a write can raise is a signal from
// outside, and it runs the handler a second time. Without this the row above
// could pass because no copy ever reached the shell at all.
func TestACopyNoWriteRaisedStillRunsTheHandler(t *testing.T) {
	if n, errOut := handledRuns(t, 3, 2); n != 2 {
		t.Errorf("the handler ran %d times, want twice: once for the write, once for the copy from outside (stderr %q)",
			n, errOut)
	}
}

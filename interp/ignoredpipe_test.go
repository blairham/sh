// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os/signal"
	"strings"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A broken pipe is answered by the signal's disposition rather than by the
// errno the kernel handed back.
//
// The write is the same write every time: into a reader that has gone. What
// differs is what the script has arranged for SIGPIPE. Left alone it kills the
// writer where it stands and nothing after the write runs. Ignored or handled,
// the kernel has no fatal signal to raise and returns EPIPE to a writer that
// is still there — and a failed write is then the ordinary failure the wording
// and the status axis already describe, with the handler delivered the way
// every other handler is.

// brokenPipe is a stream whose reader has gone: every write fails the way the
// kernel fails one into a pipe nobody holds open.
//
// A real pipeline would do it too, at the cost of writing past the buffer to
// make the race a certainty. This says the same thing in one line and says it
// about the shell's own output, which is where a handler is still in force.
type brokenPipe struct{}

func (brokenPipe) Write(p []byte) (int, error) { return 0, syscall.EPIPE }

func pipeRun(t *testing.T, src string) (errOut string, status int) {
	t.Helper()
	// A trap set at the top level is the *process's* disposition, because
	// that is what a shell at the top level is — and one test process runs
	// every test in the package. An arrangement left behind here is inherited
	// by every child a later test starts.
	t.Cleanup(func() { signal.Reset(syscall.SIGPIPE) })
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem, diag := failingWrites()
	var e bytes.Buffer
	r := newTestRunner(t, &Runner{Stdout: brokenPipe{}, Stderr: &e, Semantics: &sem, Diagnostics: &diag})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return e.String(), st
}

func TestADefaultBrokenPipeEndsTheWriter(t *testing.T) {
	errOut, _ := pipeRun(t, `echo hi; echo after >&2`)
	if errOut != "" {
		t.Errorf("stderr = %q, want nothing — the writer is dead and says so to no one", errOut)
	}
}

func TestAnIgnoredBrokenPipeLeavesTheWriterRunning(t *testing.T) {
	errOut, _ := pipeRun(t, `trap '' PIPE; echo hi; echo after >&2`)
	if !strings.Contains(errOut, "after") {
		t.Errorf("stderr = %q, want the command after the failed write to run", errOut)
	}
}

// With the signal disarmed the write is an ordinary failed write, so it is
// reported in the wording every other failed write uses rather than in one of
// its own, and it carries the same status.
func TestAnIgnoredBrokenPipeIsWordedLikeAnyFailedWrite(t *testing.T) {
	errOut, _ := pipeRun(t, `trap '' PIPE; echo hi; echo "st=$?" >&2`)
	if want := "echo: write error:"; !strings.Contains(errOut, want) {
		t.Errorf("stderr = %q, want it to carry %q", errOut, want)
	}
	if !strings.Contains(errOut, "st=1") {
		t.Errorf("stderr = %q, want the failed write to fail the command", errOut)
	}
}

// A handler is not an ignore, and it is not the default action either: the
// signal is delivered rather than fatal, so the body runs — between commands,
// where every handler runs — and the failed write is still reported.
func TestAHandledBrokenPipeRunsTheHandlerAndCarriesOn(t *testing.T) {
	errOut, _ := pipeRun(t, `trap 'echo handled >&2' PIPE; echo hi; echo after >&2`)
	for _, want := range []string{"echo: write error:", "handled", "after"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr = %q, want it to carry %q", errOut, want)
		}
	}
	if i, j := strings.Index(errOut, "handled"), strings.Index(errOut, "after"); i > j {
		t.Errorf("stderr = %q, want the handler to run before the next command", errOut)
	}
}

// The three arrangements over one shape, so the difference between dying and
// carrying on is the trap and nothing else.
func TestOnlyTheDefaultActionEndsTheWriter(t *testing.T) {
	for _, c := range []struct {
		name  string
		setup string
		dies  bool
	}{
		{"nothing arranged", ``, true},
		{"ignored", `trap '' PIPE; `, false},
		{"handled", `trap ':' PIPE; `, false},
	} {
		errOut, _ := pipeRun(t, c.setup+`echo hi; echo after >&2`)
		if got := !strings.Contains(errOut, "after"); got != c.dies {
			t.Errorf("%s: stderr = %q, writer died = %v, want %v", c.name, errOut, got, c.dies)
		}
	}
}

// An element's own broken pipe is the element's, and it is never delivered to
// the shell that started it. Nothing in the panel prints the outer handler
// here — three print the inner one and one prints neither, and the outer one
// would be a signal arriving at a process that never had it.
func TestABrokenPipeInsideAnElementIsNotTheOuterShellsToHandle(t *testing.T) {
	const src = `v=x; i=0; while [ $i -lt 17 ]; do v=$v$v; i=$((i+1)); done; ` +
		`trap 'echo outer >&2' PIPE; ` +
		`{ trap 'echo inner >&2' PIPE; echo "$v"; echo reached >&2; } | true; echo after >&2`
	errOut, _ := pipeRun(t, src)
	if strings.Contains(errOut, "outer") {
		t.Errorf("stderr = %q, want the outer handler left alone", errOut)
	}
	for _, want := range []string{"reached", "after"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr = %q, want it to carry %q", errOut, want)
		}
	}
}

// A pipeline element is where the difference between an ignore and a handler
// shows: POSIX carries an ignore across the boundary and puts a handled
// condition back to its default, so the same trap decides opposite outcomes
// for a writer standing inside one.
func TestAPipelineElementInheritsTheIgnoreAndNotTheHandler(t *testing.T) {
	// 2^17 bytes against a 64K buffer, so the write cannot fit and the
	// reader has already gone.
	const src = `v=x; i=0; while [ $i -lt 17 ]; do v=$v$v; i=$((i+1)); done; ` +
		`{ echo "$v"; echo reached >&2; } | true; echo after >&2`
	for _, c := range []struct {
		name    string
		setup   string
		carries bool
	}{
		{"nothing arranged", ``, false},
		{"an inherited ignore", `trap '' PIPE; `, true},
		{"an inherited handler", `trap ':' PIPE; `, false},
	} {
		errOut, _ := pipeRun(t, c.setup+src)
		if got := strings.Contains(errOut, "reached"); got != c.carries {
			t.Errorf("%s: stderr = %q, writer carried on = %v, want %v", c.name, errOut, got, c.carries)
		}
		if !strings.Contains(errOut, "after") {
			t.Errorf("%s: stderr = %q, want the shell after the pipeline to carry on", c.name, errOut)
		}
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// sessionWithABug is a session whose `boom` raises an interpreter bug, reading
// the given lines from something that is not a terminal.
//
// Not a terminal because that is the loop a test can drive: the editor needs
// one, and the guard is deliberately in neither loop but in the line both of
// them run.
func sessionWithABug(t *testing.T, typed string, trace bool) (out, errs *strings.Builder, status int) {
	t.Helper()
	r := newTestRunner(nil)
	r.Register("boom", func(*interp.Runner, context.Context, []string) int {
		panic("an invariant broke")
	})
	out, errs = &strings.Builder{}, &strings.Builder{}
	rd, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rd.Close() })
	go func() {
		_, _ = w.WriteString(typed)
		_ = w.Close()
	}()
	r.Stdout, r.Stderr = out, errs
	s := Shell{
		Runner: r, Dialect: syntax.Core(), In: rd, Out: out, Err: errs,
		Name: "testsh", PanicTrace: trace,
	}
	status, err = s.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return out, errs, status
}

// A session is the process, so an interpreter bug costs the line and the
// session goes on — with its variables, its functions and its directory.
//
// interp itself keeps panicking, which is right for a library. This is the
// shell around it deciding the process survives.
func TestABugOnALineCostsTheLine(t *testing.T) {
	out, errs, _ := sessionWithABug(t, "kept=yes\nboom\necho after=$? kept=$kept\n", false)

	if !strings.Contains(errs.String(), "testsh: internal error: an invariant broke") {
		t.Errorf("err = %q, want the bug reported as a diagnostic", errs)
	}
	if !strings.Contains(out.String(), "after=2 kept=yes\n") {
		t.Errorf("out = %q, want the next line run with the state it had", out)
	}
}

// The stack is drawn only where the caller asked for it. A trace at a prompt
// scrolls the session away, which is its own damage on top of the bug.
func TestTheStackIsDrawnOnlyWhenAsked(t *testing.T) {
	_, quiet, _ := sessionWithABug(t, "boom\n", false)
	if strings.Contains(quiet.String(), "goroutine ") {
		t.Errorf("err = %q, want no stack trace by default", quiet)
	}
	_, asked, _ := sessionWithABug(t, "boom\n", true)
	if !strings.Contains(asked.String(), "goroutine ") {
		t.Errorf("err = %q, want the stack trace that was asked for", asked)
	}
}

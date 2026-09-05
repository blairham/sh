// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// panicking is a shell with a builtin that raises an interpreter bug on
// demand.
//
// A registered builtin is how a test reaches the inside of a run without
// waiting for a real bug to be found: it panics on the same goroutine, inside
// the same RunPart, as anything under interp/ would.
func panicking() driver.Shell {
	sh := shell()
	sh.Register = func(r *interp.Runner) {
		r.Register("boom", func(*interp.Runner, context.Context, []string) int {
			panic("an invariant broke")
		})
	}
	return sh
}

// A panic in the interpreter costs the run and never the process.
//
// interp panics on an internal bug because it is a library, and that is
// correct. A shell cannot answer that way — the process is the session, and
// for an embedder the process is somebody else's program — so the front end
// turns the panic into what any other failure is: a diagnostic on the error
// stream and a status.
func TestAnInterpreterBugIsADiagnosticAndAStatus(t *testing.T) {
	out, errs, code := runArgs(t, panicking(), "testsh", "-c", "echo one; boom; echo two")

	if code == 0 {
		t.Errorf("status = %d, want a failing one", code)
	}
	if !strings.Contains(out, "one\n") {
		t.Errorf("out = %q, want what ran before the bug", out)
	}
	if strings.Contains(out, "two") {
		t.Errorf("out = %q, want the run to have ended at the bug", out)
	}
	// Named, so the report says which shell said it, and carrying what the
	// panic carried, which is the only thing anyone has to go on.
	for _, want := range []string{
		"testsh: internal error: an invariant broke",
		"https://github.com/blairham/sh/issues",
		"SH_PANIC_TRACE=1",
	} {
		if !strings.Contains(errs, want) {
			t.Errorf("err = %q, want it to contain %q", errs, want)
		}
	}
	// And it is a shell's diagnostic rather than the runtime's crash.
	if strings.Contains(errs, "goroutine ") {
		t.Errorf("err = %q, want no stack trace unless one was asked for", errs)
	}
}

// The trace is behind an environment variable, on every route.
//
// A stack trace printed at a prompt scrolls the session away and buries the
// line that said what happened, so it is asked for rather than given. Both
// routes are checked because they reach the guard differently — the script
// route holds one itself, and the prompt hands the answer to repl, which is a
// library and may not read the environment for itself.
func TestTheStackTraceIsAskedFor(t *testing.T) {
	t.Run("the script route", func(t *testing.T) {
		t.Setenv("SH_PANIC_TRACE", "1")
		_, errs, _ := runArgs(t, panicking(), "testsh", "-c", "boom")
		if !strings.Contains(errs, "goroutine ") {
			t.Errorf("err = %q, want the stack trace that was asked for", errs)
		}
	})

	t.Run("the prompt route", func(t *testing.T) {
		t.Setenv("SH_PANIC_TRACE", "1")
		_, errs, _ := runPipedShell(t, panicking(), "boom\n", "testsh", "-i")
		if !strings.Contains(errs, "goroutine ") {
			t.Errorf("err = %q, want the stack trace that was asked for", errs)
		}
	})
}

// At a prompt the unit is the line: one bad line costs that line, and the
// session goes on with everything it had.
func TestAnInterpreterBugAtAPromptCostsTheLine(t *testing.T) {
	typed := "kept=yes\nboom\necho after=$? kept=$kept\n"
	out, errs, code := runPipedShell(t, panicking(), typed, "testsh", "-i")

	if !strings.Contains(errs, "internal error") {
		t.Errorf("err = %q, want the bug reported", errs)
	}
	// The session survived: a variable set before the bug is still set, and
	// the line after it ran.
	if !strings.Contains(out, "after=2 kept=yes\n") {
		t.Errorf("out = %q, want the session to have kept going with its state", out)
	}
	// A line that never finished leaves a failing status rather than the one
	// before it, which is what `after=2` above is reading.
	if code != 0 {
		t.Errorf("status = %d, want the session itself to have ended cleanly", code)
	}
}

// The backstop around the session catches what happens on either side of the
// loop, where there is no line to charge it to.
//
// A prelude is the dialect's own plumbing rather than anything a person typed,
// so a bug in it is a broken shell and the session does not start — but it
// still ends as a shell ends, with a diagnostic and a status.
func TestAnInterpreterBugOutsideTheLoopEndsTheSessionAsAShell(t *testing.T) {
	sh := panicking()
	sh.Prelude = "boom\n"
	out, errs, code := runPipedShell(t, sh, "echo typed\n", "testsh", "-i")

	if code == 0 {
		t.Errorf("status = %d, want a failing one", code)
	}
	if !strings.Contains(errs, "internal error") {
		t.Errorf("err = %q, want the bug reported", errs)
	}
	if strings.Contains(out, "typed") {
		t.Errorf("out = %q, want no session on a broken dialect", out)
	}
}

// A startup file that tickles a bug costs the file and not the session.
//
// The opposite of what a parse error in the same file does, and deliberately:
// a file that will not parse is wrong, while a file that parsed and then
// broke the interpreter is the shell being wrong — and a half-configured
// prompt is a far better answer to that than no prompt at all.
func TestAnInterpreterBugInAStartupFileCostsTheFile(t *testing.T) {
	rc := filepath.Join(t.TempDir(), "rc.sh")
	if err := os.WriteFile(rc, []byte("FROM_RC=yes\nboom\nAFTER_RC=yes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ENV", rc)

	out, errs, code := runPipedShell(t, panicking(), "echo rc=$FROM_RC after=$AFTER_RC\n", "testsh", "-i")

	if !strings.Contains(errs, "internal error") {
		t.Errorf("err = %q, want the bug reported", errs)
	}
	if !strings.Contains(out, "rc=yes after=\n") {
		t.Errorf("out = %q, want a session that started with what the file managed to set", out)
	}
	if code != 0 {
		t.Errorf("status = %d, want the session to have started", code)
	}
}

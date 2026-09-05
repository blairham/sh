// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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

// A bug on one of the shell's own goroutines is caught the same way.
//
// This is the road the guard above cannot watch. A shell runs four things
// beside itself — a background job, either half of a pipeline, a coprocess, a
// process substitution — and every one of them is a goroutine here where a real
// shell forks. `recover` reaches only the goroutine that panicked, so the guard
// around the run is on the wrong one, and a bug hit on any of the four ended the
// process however carefully the caller had wrapped its call. For a shell that is
// the session; for an embedder it is somebody else's program.
//
// Each case runs a line *after* the one that panicked, and its status is read
// out. If the guard were missing, this test binary would not report a failure —
// it would die, along with every test after it.
func TestAnInterpreterBugOnAGoroutineOfItsOwnCostsOnlyThatGoroutine(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a background job", `boom & wait -n; echo "after=$?"`, "after=2\n"},
		// `cat` is the element downstream, and its own error stream is sent
		// away rather than left pointing at the shell's: a child whose
		// stderr is not a file is copied into it by os/exec, on a goroutine
		// with no share of the lock interp puts over a caller's writer, and
		// that races the report this test is here to read. It is #735 and
		// it is not this.
		{"a pipeline element", `boom | cat 2>/dev/null; echo "after=$?"`, "after=0\n"},
		{"a coprocess", `coproc boom; read -r l <&"${COPROC[0]}"; echo after`, "after\n"},
		{"a process substitution", `read -r l < <(boom); echo after`, "after\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sh := panicking()
			sh.Dialect.Coproc = true
			// Streams that can be read while the shell may still be
			// writing to them, which is the whole shape of this: the four
			// constructs above outlive the statement that started them, so
			// a report about one can land after the run has returned. A
			// plain bytes.Buffer read from here would be the caller reading
			// its own writer with no lock on it — an embedder whose shell
			// runs background work owes it the same courtesy.
			var o, e syncBuffer
			sh.Stdout, sh.Stderr = &o, &e
			// With a deadline, because the failure these are written to
			// catch is a shell that never comes back — and left to the
			// package's own timeout that is ten minutes and a goroutine
			// dump naming whichever test was running at the time.
			finished := make(chan int, 1)
			go func() { finished <- driver.MainArgs(sh, []string{"testsh", "-c", tc.src}) }()
			var code int
			select {
			case code = <-finished:
			case <-time.After(30 * time.Second):
				t.Fatal("the shell never came back — something it was waiting " +
					"on was never handed over")
			}
			out, errs := o.String(), e.String()

			if out != tc.want {
				t.Errorf("out = %q, want %q — the shell went on after the bug", out, tc.want)
			}
			// Reported, and reported as a bug in the shell, exactly as one
			// on the calling goroutine is. It is the only sign a person
			// gets that anything happened at all.
			for _, want := range []string{
				"testsh: internal error: an invariant broke",
				"https://github.com/blairham/sh/issues",
			} {
				if !strings.Contains(errs, want) {
					t.Errorf("err = %q, want it to contain %q", errs, want)
				}
			}
			if strings.Contains(errs, "goroutine ") {
				t.Errorf("err = %q, want no stack trace unless one was asked for", errs)
			}
			// The run itself ended the way the script did rather than the
			// way the bug did: nothing on the shell's own goroutine
			// panicked, so there is nothing for the guard around the route
			// to report and no status of its own to give.
			if code != 0 {
				t.Errorf("status = %d, want the run to have finished", code)
			}
		})
	}
}

// syncBuffer is a caller's stream that is safe to read while the shell it was
// handed to may still be writing to it.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

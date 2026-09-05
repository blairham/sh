// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The end of input is a request to leave, and a session with a job stopped
// answers it the way `exit` does: it says so and stays, and the next one goes
// through.
//
// Driven down a file rather than through a terminal, because what is being
// checked is the loop and not the line discipline: the same call is made from
// both loops, which is what keeps them from disagreeing about it.
func TestTheEndOfInputIsHeldBackForAStoppedJob(t *testing.T) {
	sh, errs := stoppedSession(t, "")
	if _, err := sh.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	// Once, and once only: a session that warned at every read would never
	// end, which is the failure on the other side of this.
	if got := strings.Count(errs.String(), "stopped jobs"); got != 1 {
		t.Errorf("warned %d times, want once: %q", got, errs.String())
	}
}

// A session with nothing stopped ends at the first end of input, which is what
// every session that has never suspended anything is.
func TestTheEndOfInputEndsASessionWithNothingStopped(t *testing.T) {
	sh, errs := plainSession(t, "")
	if _, err := sh.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(errs.String(), "stopped jobs") {
		t.Errorf("said %q, want nothing about stopped jobs", errs.String())
	}
}

// And the warning is the interpreter's to word: a session whose dialect says
// nothing about a stopped job at exit leaves at the first end of input.
func TestADialectThatDoesNotHoldEndsAtOnce(t *testing.T) {
	sh, errs := stoppedSession(t, "")
	sem := interp.PosixSemantics()
	sem.StoppedJobsHoldTheExit = interp.No
	sh.Runner.Semantics = &sem
	if _, err := sh.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(errs.String(), "stopped jobs") {
		t.Errorf("said %q, want nothing", errs.String())
	}
}

// stoppedSession is a session that already has a job stopped, built the way a
// ^Z builds one: an external command the shell's own wait reports as stopped
// rather than finished.
func stoppedSession(t *testing.T, script string) (Shell, *strings.Builder) {
	t.Helper()
	sh, errs := plainSession(t, script)
	sh.Runner.JobControl = true
	sh.Runner.WaitForCommand = func(int) (interp.Wait, error) {
		return interp.Wait{Signal: syscall.SIGTSTP, Stopped: true}, nil
	}
	f, err := syntax.Parse("/usr/bin/true", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if err := sh.Runner.RunPart(t.Context(), f); err != nil {
		t.Fatal(err)
	}
	jobs := sh.Runner.Jobs()
	if len(jobs) != 1 || !jobs[0].Stopped {
		t.Fatalf("jobs = %+v, want one stopped job to leave behind", jobs)
	}
	t.Cleanup(func() {
		// The wait was a fake, so the process is real and still running.
		if pid := jobs[0].PID; pid > 0 {
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		}
	})
	return sh, errs
}

// plainSession reads its lines from a file, which is the loop without an
// editor — no terminal is needed to ask what the loop does at the end of its
// input.
func plainSession(t *testing.T, script string) (Shell, *strings.Builder) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "script")
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = in.Close() })
	r := newTestRunner(nil)
	r.Dir = dir
	sem := interp.PosixSemantics()
	sem.StoppedJobsHoldTheExit = interp.Yes
	r.Semantics = &sem
	dg := interp.Diagnostics{StoppedJobsAtExit: "there are stopped jobs"}
	r.Diagnostics = &dg
	errs := &strings.Builder{}
	r.Stderr = errs
	return Shell{
		Runner: r, Dialect: syntax.Core(), In: in,
		Out: &strings.Builder{}, Err: errs, Name: "testsh",
	}, errs
}

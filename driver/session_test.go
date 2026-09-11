// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// A session is the fourth way into this front end, beside `-c`, a script file
// and a prompt, and the one thing it does that none of the others do is stay
// open. What these tests hold down is that "stays open" means the shell rather
// than the text: the variables, the functions, the traps and the directory.

// session opens one in a directory of its own, with its output captured.
//
// The directory is not a nicety. A Runner with no Dir treats every relative
// path as relative to wherever the test process happens to be, so a redirect
// in one of these inputs would write into the source tree.
func session(t *testing.T) (*driver.Session, *strings.Builder, *strings.Builder) {
	t.Helper()
	var out, errs strings.Builder
	dir := t.TempDir()
	sh := driver.Shell{Name: "sh", Dir: dir, Stdout: &out, Stderr: &errs, KeepProcess: true}
	// One axis answered, the way the other tests here answer the ones they are
	// not about: an EXIT trap whose body a session runs at close asks whether a
	// trap body runs the part of it that parsed, and a test about *when* the
	// trap fires must not be deciding that question by leaving it unanswered.
	sh.Semantics.TrapBodyRunsWhatParsed = interp.Yes
	s, code := driver.NewSession(sh)
	if code != 0 || s == nil {
		t.Fatalf("NewSession: status %d", code)
	}
	return s, &out, &errs
}

// The whole point of the type: what one input set, the next one sees.
func TestASessionKeepsWhatAnInputSet(t *testing.T) {
	t.Parallel()
	s, out, errs := session(t)

	if status := s.Run(t.Context(), "x=hello"); status != 0 {
		t.Fatalf("status = %d, stderr = %q", status, errs.String())
	}
	if status := s.Run(t.Context(), "echo $x"); status != 0 {
		t.Fatalf("status = %d, stderr = %q", status, errs.String())
	}
	if got := out.String(); got != "hello\n" {
		t.Errorf("output = %q, want %q — the second input got a fresh shell", got, "hello\n")
	}
}

func TestAFunctionDefinedInOneInputSurvivesToTheNext(t *testing.T) {
	t.Parallel()
	s, out, errs := session(t)

	s.Run(t.Context(), "greet() { echo hi; }")
	if status := s.Run(t.Context(), "greet"); status != 0 {
		t.Fatalf("status = %d, stderr = %q", status, errs.String())
	}
	if got := out.String(); got != "hi\n" {
		t.Errorf("output = %q, want %q", got, "hi\n")
	}
}

// A session runs in the directory it was given rather than the process's, and
// a relative path inside it resolves there. This is the reason Shell.Dir
// exists: one process, several sessions, each with a directory of its own.
func TestASessionRunsInTheDirectoryItWasGiven(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var out, errs strings.Builder
	s, code := driver.NewSession(driver.Shell{
		Name: "sh", Dir: dir, Stdout: &out, Stderr: &errs, KeepProcess: true,
	})
	if code != 0 {
		t.Fatalf("NewSession: status %d", code)
	}
	if status := s.Run(t.Context(), "echo written > made-here"); status != 0 {
		t.Fatalf("status = %d, stderr = %q", status, errs.String())
	}
	b, err := os.ReadFile(filepath.Join(dir, "made-here"))
	if err != nil {
		t.Fatalf("the redirect did not write into the session's directory: %v", err)
	}
	if string(b) != "written\n" {
		t.Errorf("file = %q", b)
	}
	if got := strings.TrimSpace(pwdOf(t, s)); got != dir {
		t.Errorf("pwd = %q, want %q", got, dir)
	}
}

// pwdOf asks the session where it is, through the shell rather than through
// the Go value, so the answer is the one a script would get.
func pwdOf(t *testing.T, s *driver.Session) string {
	t.Helper()
	var out strings.Builder
	// A second session would have its own runner, so the question has to go to
	// this one; its stdout is the builder the caller gave it, so read what the
	// command adds rather than replacing the writer.
	before := s.Runner().Stdout
	if b, ok := before.(*strings.Builder); ok {
		mark := b.Len()
		s.Run(t.Context(), "pwd")
		return b.String()[mark:]
	}
	s.Run(t.Context(), "pwd")
	return out.String()
}

// A `cd` in one input moves the session, which is what makes it a session
// rather than a series of unrelated commands — and it moves r.Dir rather than
// the process, so another session is unaffected.
func TestACdMovesTheSessionAndNotTheProcess(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sub := filepath.Join(dir, "inner")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	var outA, outB strings.Builder
	a, _ := driver.NewSession(driver.Shell{Name: "sh", Dir: dir, Stdout: &outA, KeepProcess: true})
	b, _ := driver.NewSession(driver.Shell{Name: "sh", Dir: dir, Stdout: &outB, KeepProcess: true})

	a.Run(t.Context(), "cd inner")
	a.Run(t.Context(), "pwd")
	b.Run(t.Context(), "pwd")

	if got := strings.TrimSpace(outA.String()); got != sub {
		t.Errorf("the session that moved reports %q, want %q", got, sub)
	}
	if got := strings.TrimSpace(outB.String()); got != dir {
		t.Errorf("the other session reports %q, want %q — a cd moved the process", got, dir)
	}
}

// The EXIT trap fires once, when the session closes, and not after every
// input. Running it per input would run it as many times as a client sends a
// prompt, which is a trap firing for something that has not happened.
func TestTheExitTrapFiresOnceAtClose(t *testing.T) {
	t.Parallel()
	s, out, _ := session(t)

	s.Run(t.Context(), "trap 'echo bye' EXIT")
	s.Run(t.Context(), "echo one")
	s.Run(t.Context(), "echo two")
	if got := out.String(); strings.Contains(got, "bye") {
		t.Fatalf("output = %q — the trap fired before the session closed", got)
	}
	s.Close(t.Context())
	if got, want := out.String(), "one\ntwo\nbye\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// A status is the last command's, per input.
func TestRunReportsTheStatusOfTheInput(t *testing.T) {
	t.Parallel()
	s, _, _ := session(t)

	if status := s.Run(t.Context(), "true"); status != 0 {
		t.Errorf("status = %d, want 0", status)
	}
	if status := s.Run(t.Context(), "exit 7"); status != 7 {
		t.Errorf("status = %d, want 7", status)
	}
}

// `exit` ends the shell, and the session says so rather than quietly running
// the next input in a shell that is not there.
func TestExitedIsReportedAfterAnExit(t *testing.T) {
	t.Parallel()
	s, _, _ := session(t)

	if s.Exited() {
		t.Fatal("a session reports as exited before anything ran")
	}
	s.Run(t.Context(), "exit 3")
	if !s.Exited() {
		t.Error("the session did not notice `exit`")
	}
}

// A parse failure is one bad input rather than the end of the shell: the
// session is still there and still holds what it held.
func TestAParseFailureDoesNotEndTheSession(t *testing.T) {
	t.Parallel()
	s, out, errs := session(t)

	s.Run(t.Context(), "x=kept")
	if status := s.Run(t.Context(), "if"); status == 0 {
		t.Error("a failed parse reported success")
	}
	if errs.Len() == 0 {
		t.Error("nothing was said about the failed parse")
	}
	if s.Exited() {
		t.Fatal("a failed parse ended the session")
	}
	s.Run(t.Context(), "echo $x")
	if got := out.String(); got != "kept\n" {
		t.Errorf("output = %q, want the variable to have survived", got)
	}
}

// An input is run the way `-c` runs a command string, and the proof is that
// the two routes word the same failure identically. A session that reported a
// parse failure its own way would be a second front end, which is the thing
// this type exists inside driver to avoid.
func TestAnInputIsWordedAsACommandStringIs(t *testing.T) {
	t.Parallel()
	var oneShot strings.Builder
	driver.RunCommand(driver.Shell{
		Name: "sh", Dir: t.TempDir(), Stdout: &oneShot, Stderr: &oneShot, KeepProcess: true,
	}, "if", nil)

	s, _, errs := session(t)
	s.Run(t.Context(), "if")

	if got, want := errs.String(), oneShot.String(); got != want {
		t.Errorf("session said %q, the -c route said %q", got, want)
	}
	if errs.Len() == 0 {
		t.Error("neither route said anything about a program that will not parse")
	}
}

// Canceling an input's context stops it, and the session survives to run the
// next one. This is what session/cancel becomes.
func TestCancelingAnInputLeavesTheSessionUsable(t *testing.T) {
	t.Parallel()
	s, out, _ := session(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	s.Run(ctx, "echo never")

	s.Run(t.Context(), "echo after")
	if got := out.String(); !strings.Contains(got, "after") {
		t.Errorf("output = %q, want the session to have run the next input", got)
	}
}

// A run-time option can change the grammar, and a session has to read the
// runner's dialect rather than the shell's when it parses the next input:
// `set -o` in one turn governs the parse of the one after it. Reading the
// shell's would parse the next input with a configuration the shell has
// already moved off, which is a syntax error for a construct that is now
// legal.
//
// The flag is named rather than a shell, and the builtin that flips it is
// registered here, because which builtin flips it under which name is a
// dialect's business.
func TestAGrammarChangeInOneInputReachesTheNext(t *testing.T) {
	t.Parallel()
	var out, errs strings.Builder
	s, code := driver.NewSession(driver.Shell{
		Name: "sh", Dir: t.TempDir(), Stdout: &out, Stderr: &errs, KeepProcess: true,
		Register: func(r *interp.Runner) {
			r.Register("groups-on", func(r *interp.Runner, _ context.Context, _ []string) int {
				r.SetMatchOption(interp.QuantifiedGroupsEverywhere, true)
				return 0
			})
		},
	})
	if code != 0 {
		t.Fatalf("NewSession: status %d", code)
	}

	// The core has no quantified groups, so this input is a syntax error
	// until the toggle has run.
	const groups = "case ab in @(ab|cd)) echo hit;; esac\n"
	if status := s.Run(t.Context(), groups); status == 0 {
		t.Errorf("status = %d before the toggle, want a parse failure", status)
	}
	if status := s.Run(t.Context(), "groups-on"); status != 0 {
		t.Fatalf("the toggle failed: status %d, stderr %q", status, errs.String())
	}
	if status := s.Run(t.Context(), groups); status != 0 {
		t.Fatalf("status = %d after the toggle, stderr %q", status, errs.String())
	}
	if got := strings.TrimSpace(out.String()); got != "hit" {
		t.Errorf("output = %q, want hit", got)
	}
}

// A job a session started is a job the session can signal, which is the same
// shell the same script gets through `-c`.
//
// The bug was a seam rather than a signal: the hook that reaches a process
// group was installed only for a front end that lets `exec` replace the
// process, so a session — which must not be replaced, and says so with
// KeepProcess — had no way to reach a group at all. With the monitor on, a
// background job leads one, so `kill %1` answered `No such process` for a job
// `jobs` had just listed, while the identical line under `-c` signaled it
// (#1814).
//
// The monitor is turned on in the script on purpose: with it off a background
// job shares this shell's group and `%1` is signaled as a process, which
// works either way and would pass whatever the hook was. This asks the
// question at the place the two routes really differ.
func TestASessionCanSignalAJobItStarted(t *testing.T) {
	t.Parallel()
	// Guarded writers rather than the helper's plain builders, because this
	// is the one test here that leaves a *background job* running while it
	// reads: the job writes on a goroutine of its own, which is the case
	// cmd/sh installs its own guarded streams for. A plain builder is a data
	// race the moment the two meet, and the race detector says so.
	var out, errs lockedWriter
	sh := driver.Shell{Name: "sh", Dir: t.TempDir(), Stdout: &out, Stderr: &errs, KeepProcess: true}
	// The axis the script turns on, answered here because a session has no
	// terminal and the panel disagrees about what `set -m` does then: the
	// core refuses it as unanswered, which would leave the job in this
	// shell's group and the test measuring the case that works either way.
	sh.Semantics.MonitorNeedsATerminal = interp.No
	s, code := driver.NewSession(sh)
	if code != 0 || s == nil {
		t.Fatalf("NewSession: status %d", code)
	}

	// -0 delivers nothing, so what is measured is whether the target was
	// reachable rather than what a signal did to it; the job is then ended so
	// the test leaves nothing running.
	status := s.Run(t.Context(), `set -m; sleep 30 & kill -0 %1; echo "probe=$?"; kill %1`)
	if status != 0 {
		t.Fatalf("status = %d, stderr = %q", status, errs.String())
	}
	if got := out.String(); !strings.Contains(got, "probe=0") {
		t.Errorf("output = %q, want probe=0 — the job could not be named", got)
	}
	if got := errs.String(); strings.Contains(got, "No such process") {
		t.Errorf("stderr = %q: a job this shell had just started was reported missing", got)
	}
}

// lockedWriter is a writer two goroutines may use, which is what a shell with
// a background job in it needs.
type lockedWriter struct {
	mu sync.Mutex
	b  strings.Builder
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func (w *lockedWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

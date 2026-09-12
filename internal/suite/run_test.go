// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A suite file is not a script that happens to be shell. It starts shells,
// backgrounds them and waits for them, and a good part of what it tests is
// what happens when one does not come back — so these are about the kill and
// about who gets blamed for it.

// fakeShell writes an executable standing in for a shell, running the body
// given with the file it was handed as $1.
func fakeShell(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func testDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	tests := filepath.Join(dir, "tests")
	if err := os.MkdirAll(tests, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(tests, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return tests
}

// TestATimedOutRunIsKilledAsAGroup is the difference between a bound and a
// suggestion.
//
// Killing the shell at the top is not enough to make the wait return: a
// background job it started still holds the output pipe open, so a harness
// that killed only the leader waits for the grandchild nobody named. The file
// here backgrounds a long sleep and then sleeps itself, which is the shape
// half a suite's job-control files have.
func TestATimedOutRunIsKilledAsAGroup(t *testing.T) {
	tests := testDir(t, map[string]string{"hang.tests": "sleep 60 &\nsleep 60\n"})
	start := time.Now()
	out := runFile(context.Background(), "/bin/sh", tests, "hang.tests",
		environ(Suite{}, tests, "/bin/sh"), 500*time.Millisecond)
	elapsed := time.Since(start)
	if !out.TimedOut {
		t.Fatal("a file that sleeps for a minute was not reported as timed out")
	}
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("the kill took %v; a backgrounded job held the pipe and only a "+
			"group kill closes it", elapsed)
	}
}

func TestARunThatFinishesCarriesItsStatusAndBothStreams(t *testing.T) {
	tests := testDir(t, map[string]string{
		"ok.tests": "echo out\necho err >&2\nexit 3\n",
	})
	out := runFile(context.Background(), "/bin/sh", tests, "ok.tests",
		environ(Suite{}, tests, "/bin/sh"), 10*time.Second)
	if out.TimedOut {
		t.Fatal("a file that exits promptly was reported as hung")
	}
	if out.Status != 3 {
		t.Errorf("status %d, want 3", out.Status)
	}
	// One stream, because the suite's own ordering between them is part of
	// what is being compared: a diagnostic that moves says something
	// different about the shell.
	if !strings.Contains(out.Output, "out") || !strings.Contains(out.Output, "err") {
		t.Errorf("stdout and stderr are not both here: %q", out.Output)
	}
}

// TestTheShellVariablePointsAtTheShellOfTheRun guards the trap that would
// make this instrument grade nothing while looking green.
//
// A suite re-enters its own shell constantly. If the variable naming that
// shell were frozen to the reference, our column would run the reference
// everywhere the file recursed — the whole run would agree, and nothing would
// have been measured.
func TestTheShellVariablePointsAtTheShellOfTheRun(t *testing.T) {
	s := Suite{ShellVar: "THIS_SH"}
	for _, shell := range []string{"/bin/ours", "/bin/theirs"} {
		env := environ(s, t.TempDir(), shell)
		want := "THIS_SH=" + shell
		found := false
		for _, e := range env {
			if e == want {
				found = true
			}
		}
		if !found {
			t.Errorf("running under %s, the environment does not say %q: %v", shell, want, env)
		}
	}
}

func TestEnvironIsTheSameBothWaysBarTheShell(t *testing.T) {
	s := Suite{ShellVar: "THIS_SH"}
	dir := t.TempDir()
	a, b := environ(s, dir, "/bin/a"), environ(s, dir, "/bin/b")
	if len(a) != len(b) {
		t.Fatalf("the two runs get different environments: %v vs %v", a, b)
	}
	differ := 0
	for i := range a {
		if a[i] != b[i] {
			differ++
		}
	}
	if differ != 1 {
		t.Errorf("%d variables differ between the two runs, want exactly the one naming the shell", differ)
	}
}

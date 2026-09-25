// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"
	"os"
	"path/filepath"
	"slices"
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
	out := runFile(context.Background(), Suite{}, "/bin/sh", tests, "hang.tests",
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
	out := runFile(context.Background(), Suite{}, "/bin/sh", tests, "ok.tests",
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

// A suite whose files a shell does not run is invoked through the suite's own
// driver, and the file becomes the driver's argument.
//
// The assertion is on the order as much as on the membership: `+Z -f` are the
// shell's options and have to precede the script, and the file has to follow
// it. A vector with the same five strings in another order is a different
// command and two of the orders are silently wrong rather than refused —
// `zsh ztst.zsh +Z -f file` hands the driver three arguments and runs no test.
func TestADriverIsInvokedBeforeTheFileAndAfterTheShellsOptions(t *testing.T) {
	got := argv(Suite{Driver: "ztst.zsh", DriverArgs: []string{"+Z", "-f"}}, "A01grammar.ztst")
	want := []string{"+Z", "-f", "./ztst.zsh", "./A01grammar.ztst"}
	if !slices.Equal(got, want) {
		t.Fatalf("argv with a driver = %q, want %q", got, want)
	}
}

// A column with no driver keeps the vector it has always had. bash's suite and
// our own are both this shape, so a regression here is a regression in every
// built column at once.
func TestWithoutADriverTheShellIsHandedTheFile(t *testing.T) {
	got := argv(Suite{}, "glob.tests")
	want := []string{"./glob.tests"}
	if !slices.Equal(got, want) {
		t.Fatalf("argv with no driver = %q, want %q", got, want)
	}
}

// The shell lands where the suite's files reach for it, and it is the shell of
// *this* run.
//
// The second half is the one worth a test. A link that pointed at one shell
// for both runs would have the column grading the reference against itself and
// reporting perfect agreement — the shape this repository has lost most to,
// and one that looks like success from every direction.
func TestThePlacedShellIsThisRunsShell(t *testing.T) {
	run := t.TempDir()
	s := Suite{ShellAt: "../Src/zsh"}
	if err := placeShell(s, filepath.Join(run, "t"), "/bin/sh"); err != nil {
		t.Fatalf("placing the shell: %v", err)
	}
	at := filepath.Join(run, "Src", "zsh")
	got, err := os.Readlink(at)
	if err != nil {
		t.Fatalf("reading the link the files would follow: %v", err)
	}
	if got != "/bin/sh" {
		t.Fatalf("the placed shell is %q, want the shell this run was given", got)
	}
}

// A column that names no path for its shell has nothing placed, and placing
// something anyway would put a binary inside a suite that never asked for one.
func TestWithoutAShellPathNothingIsPlaced(t *testing.T) {
	run := t.TempDir()
	if err := placeShell(Suite{}, filepath.Join(run, "t"), "/bin/sh"); err != nil {
		t.Fatalf("a column with no ShellAt should be a no-op, got %v", err)
	}
	entries, err := os.ReadDir(run)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("placing nothing left %d entries behind", len(entries))
	}
}

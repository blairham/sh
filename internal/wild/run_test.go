// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package wild_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blairham/sh/internal/wild"
)

// shellThat writes a shell script that behaves as body says and returns its
// path, for use as one of the two shells being compared. It receives the
// script to run as $1, exactly as a real shell does.
func shellThat(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// Two shells that behave alike agree, however many scripts they are given.
func TestIdenticalShellsAgree(t *testing.T) {
	dir := t.TempDir()
	script := write(t, dir, "s", "#!/bin/sh\necho hi\n")

	rep := wild.RunSweep(context.Background(), []string{script}, "/bin/sh", "/bin/sh", 10*time.Second)
	if rep.Ran != len(wild.Probes) || rep.Agreed != rep.Ran || len(rep.Mismatches) != 0 {
		t.Errorf("%+v", rep)
	}
}

// And a difference in *either* the output or the status is reported.
func TestADifferenceIsReported(t *testing.T) {
	dir := t.TempDir()
	script := write(t, dir, "s", "#!/bin/sh\necho hi\n")

	for _, tc := range []struct{ name, body string }{
		{"different output", `echo different`},
		{"different status", `sh "$@"; exit 7`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			odd := shellThat(t, "odd", tc.body)
			rep := wild.RunSweep(context.Background(), []string{script}, odd, "/bin/sh", 10*time.Second)
			if len(rep.Mismatches) != len(wild.Probes) {
				t.Errorf("got %d mismatches, want %d: %+v", len(rep.Mismatches), len(wild.Probes), rep)
			}
		})
	}
}

// A script that waits is not evidence about either shell, so it is counted
// apart rather than reported as a difference.
func TestAScriptThatWaitsIsNotAMismatch(t *testing.T) {
	dir := t.TempDir()
	script := write(t, dir, "s", "#!/bin/sh\nsleep 30\n")

	start := time.Now()
	rep := wild.RunSweep(context.Background(), []string{script}, "/bin/sh", "/bin/sh", 300*time.Millisecond)
	if rep.Timedout != len(wild.Probes) || len(rep.Mismatches) != 0 || rep.Ran != 0 {
		t.Errorf("%+v", rep)
	}
	if elapsed := time.Since(start); elapsed > 20*time.Second {
		t.Errorf("took %v, so the timeout is not being applied", elapsed)
	}
}

// The sweep contains what it runs: a fresh directory each time, and no
// standard input to wait on.
func TestARunIsContained(t *testing.T) {
	dir := t.TempDir()
	mark := filepath.Join(dir, "escaped")
	// Writes a file in the working directory, and reports what it found
	// there — which must be nothing from the run before it.
	script := write(t, dir, "s", "#!/bin/sh\nls | tr '\\n' ' '\ntouch ran\n")

	rep := wild.RunSweep(context.Background(), []string{script}, "/bin/sh", "/bin/sh", 10*time.Second)
	if rep.Agreed != rep.Ran {
		t.Fatalf("%+v", rep)
	}
	if _, err := os.Stat(mark); err == nil {
		t.Error("a run wrote into the directory it was launched from")
	}
	if _, err := os.Stat(filepath.Join(dir, "ran")); err == nil {
		t.Error("a run wrote beside the script rather than in its own directory")
	}
	// Reading standard input ends rather than waiting, which the timeout
	// would otherwise have to catch.
	reader := write(t, dir, "r", "#!/bin/sh\nread x; echo \"[$x]\"\n")
	rep = wild.RunSweep(context.Background(), []string{reader}, "/bin/sh", "/bin/sh", 2*time.Second)
	if rep.Timedout != 0 {
		t.Errorf("reading standard input waited: %+v", rep)
	}
}

// The shell's own path appears in its diagnostics and the two shells have
// different ones, so it is replaced — but only the whole path. Replacing the
// base name as well turned `GNU bashbug` into `GNU <shell>bug` and reported
// two identical outputs as a difference.
func TestNormalisingDoesNotEatTheScriptsOwnWords(t *testing.T) {
	dir := t.TempDir()
	// Prints a word that contains the reference shell's base name.
	script := write(t, dir, "s", "#!/bin/sh\necho 'GNU shbug 1.0'\n")

	rep := wild.RunSweep(context.Background(), []string{script}, "/bin/sh", "/bin/sh", 10*time.Second)
	if len(rep.Mismatches) != 0 {
		t.Errorf("a word containing the shell's name was reported as a difference: %+v", rep.Mismatches)
	}
	// And the path *is* replaced, which is what the normalising is for: two
	// shells whose diagnostics name themselves must still compare equal.
	odd := shellThat(t, "sh", `echo "$0: bad"; exit 2`)
	other := shellThat(t, "sh2", `echo "$0: bad"; exit 2`)
	rep = wild.RunSweep(context.Background(), []string{script}, odd, other, 10*time.Second)
	if len(rep.Mismatches) != 0 {
		t.Errorf("two shells naming themselves were reported as different: %+v", rep.Mismatches)
	}
}

// A shell named by a bare word must not have that word struck out of the
// scripts' output.
//
// The reference shell is named on the command line and defaults to `bash`, so
// normalise was handed a bare word and replaced every occurrence of it — a
// word that appears in a great many scripts' own output for reasons that have
// nothing to do with which shell is running. `GNU bashbug` became
// `GNU <shell>bug` on one side and stayed as it was on the other, and two
// identical outputs were reported as a difference.
//
// The two sides have to be *named* differently for this to bite, which is
// exactly the sweep's own arrangement: ours is a built binary's path and the
// reference is whatever was typed on the command line. Here they are one
// shell under two names, so anything they disagree about is the harness.
func TestAShellsNameIsNotStruckOutOfTheOutput(t *testing.T) {
	dir := t.TempDir()
	// The script says the reference's name as part of a longer word, which is
	// what a bare-word replacement would cut in half.
	script := write(t, dir, "s", "#!/bin/sh\necho 'GNU shbug'\n")

	rep := wild.RunSweep(context.Background(), []string{script}, "/bin/sh", "sh", 10*time.Second)
	if rep.Ran == 0 {
		t.Skip("sh did not run here")
	}
	if rep.Agreed != rep.Ran || len(rep.Mismatches) != 0 {
		t.Errorf("one shell under two names disagreed with itself: %+v", rep.Mismatches)
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// cannotExecute is the status a shell reports for a command it found and could
// not run, and in this file it can only mean the machine.
//
// Every case here runs `sh` and asserts a status the *signal* made — 141, 143,
// 129 and their base-256 twins, or an ordinary 0 and 3. None of them is 126,
// and none of them can become 126 by being wrong: the encoding this file is
// about happens after a process has died, so a 126 means no process ever
// lived. A fork the machine had no room for, an image the kernel would not
// take — and neither is a measurement of anything this file asks about.
const cannotExecute = 126

// How many times a run that never started is asked for again, and how long the
// pauses are.
//
// Bounded, and the last answer is kept: a machine that genuinely cannot run
// `sh` has to fail this file rather than be waited for, and it has to fail
// saying so.
const (
	deathRunAttempts = 6
	deathRunPause    = 150 * time.Millisecond
)

// deathRun runs one line and returns what the shell printed and the status.
//
// **A run that never started is asked for again rather than reported as a
// wrong status** (#1342). This matters far past this file: these tests fail
// under the parallel load this repository generates when several `go test
// ./interp/` runs share a machine, and because they fail during *mutation*
// runs they manufacture **false kills** — the dangerous direction, because a
// false kill reads as "the tests already cover this" and closes the question.
// One mutation round scored 18/19 with four of them, this test being the only
// failure for all four mutants; re-taken without it, six genuine survivors
// appeared. On #1261 a mutant came back KILLED solely because of this test and
// was only reclassified because the agent was skeptical.
//
// The signal assertions are untouched and nothing here is loosened: a status
// that is wrong in any way a signal could make it wrong still fails, and so
// does a machine that cannot run `sh` at all — the difference is that the
// failure now names which of the two happened and quotes what the shell said
// about it. A test that fails under load may be the only thing telling you
// something breaks under load, so the load-sensitive thing is refused a
// verdict rather than given a lenient one.
func deathRun(t *testing.T, a Answer, src string) (string, int) {
	t.Helper()
	out, st, tries := deathRunPersisting(t, a, src, testPATH())
	if st == cannotExecute {
		t.Fatalf("`%s` never ran, %d attempts over, so nothing here was measured — this is the machine and not the shell: %q",
			src, tries, out)
	}
	return out, st
}

// deathRunPersisting is deathRun without the verdict, so that the giving up can
// be tested rather than described. It also reports how many attempts it took.
func deathRunPersisting(t *testing.T, a Answer, src string, env []string) (string, int, int) {
	t.Helper()
	var out string
	var st int
	tries := 0
	for tries < deathRunAttempts {
		out, st = deathRunOnce(t, a, src, env)
		tries++
		if st != cannotExecute {
			break
		}
		if tries < deathRunAttempts {
			time.Sleep(deathRunPause)
		}
	}
	return out, st, tries
}

func deathRunOnce(t *testing.T, a Answer, src string, env []string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := permissive()
	sem.SignalDeathStatusIsTwoFiftySix = a
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "testsh", Env: env})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return buf.String(), st
}

// A killed process has no exit status of its own. Go reports -1, which is not
// a status a shell can report at all — it reached `$?` as -1 and the process's
// own exit code as 255.
//
// PIPE and TERM rather than KILL: both are ordinary here, and using two says
// the base is a base rather than something done for one signal.
func TestAKilledCommandEncodesTheSignal(t *testing.T) {
	for _, c := range []struct {
		sig            string
		oneTwentyEight int
		twoFiftySix    int
	}{
		{"PIPE", 141, 269},
		{"TERM", 143, 271},
		{"HUP", 129, 257},
	} {
		src := "sh -c 'kill -" + c.sig + " $$'"
		if out, st := deathRun(t, No, src); st != c.oneTwentyEight {
			t.Errorf("%s with base 128: status %d, want %d (the shell said %q)", c.sig, st, c.oneTwentyEight, out)
		}
		if out, st := deathRun(t, Yes, src); st != c.twoFiftySix {
			t.Errorf("%s with base 256: status %d, want %d (the shell said %q)", c.sig, st, c.twoFiftySix, out)
		}
	}
}

// A command that exits by itself is untouched by any of this, whichever way
// the axis is answered — which is what makes the encoding about being killed
// rather than about failing.
func TestAnOrdinaryFailureIsNotEncoded(t *testing.T) {
	for _, a := range []Answer{No, Yes, Unspecified} {
		if out, st := deathRun(t, a, `sh -c 'exit 3'`); st != 3 {
			t.Errorf("answer %v: status %d (%q), want 3", a, st, out)
		}
	}
	// Nor is success, which would otherwise be the easiest thing to break by
	// encoding signal 0.
	for _, a := range []Answer{No, Yes} {
		if _, st := deathRun(t, a, `sh -c 'exit 0'`); st != 0 {
			t.Errorf("answer %v: status %d, want 0", a, st)
		}
	}
}

// Unanswered is refused, like any other axis: the two bases give different
// answers to `$?` and the core does not pick one.
func TestAnUnansweredBaseIsRefused(t *testing.T) {
	out, _ := deathRun(t, Unspecified, `sh -c 'kill -PIPE $$'`)
	if !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("said %q, want a refusal", out)
	}
}

// unrunnableShell is an `sh` on a PATH of its own that the kernel will not
// execute, which is what a machine with nothing left to fork looks like from
// in here: a status of 126 and a line on standard error, with no process
// having run. The script kills itself with PIPE once it is allowed to run, so
// the measurement the file is actually about is still available on the other
// side of the refusal.
func unrunnableShell(t *testing.T) (dir, path string) {
	t.Helper()
	dir = t.TempDir()
	path = filepath.Join(dir, "sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nkill -PIPE $$\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, path
}

// TestARunThatNeverStartedIsAskedForAgain is the fix for #1342, and the shape
// of the failure it removes.
//
// The subject cannot be executed when the first attempt is made and can be by
// the time a later one is, which is the load the real thing is: the machine
// had no room and then it had some. What must not happen is the 126 reaching
// an assertion about signal encoding, because there it reads as "the status is
// wrong" — a verdict about the code, taken from a fact about the machine.
func TestARunThatNeverStartedIsAskedForAgain(t *testing.T) {
	dir, path := unrunnableShell(t)

	go func() {
		time.Sleep(deathRunPause + 50*time.Millisecond)
		_ = os.Chmod(path, 0o700)
	}()

	out, st, tries := deathRunPersisting(t, No, `sh -c 'kill -PIPE $$'`, []string{"PATH=" + dir})
	if st != 141 {
		t.Fatalf("status %d after %d attempt(s), want the 141 the signal makes (the shell said %q)", st, tries, out)
	}
	if tries < 2 {
		t.Errorf("it got there on attempt %d, so the run that could not start never happened and this proves nothing", tries)
	}
}

// TestARunThatNeverStartsAtAllIsStillAFailure is the other half, and it is what
// keeps the retry from becoming a way of not noticing.
//
// A subject that will never run is a real failure and has to come back as one,
// bounded, carrying what the shell said — the same bargain the startup gate
// makes with an exec the kernel refuses for good.
func TestARunThatNeverStartsAtAllIsStillAFailure(t *testing.T) {
	dir, _ := unrunnableShell(t)

	out, st, tries := deathRunPersisting(t, No, `sh -c 'kill -PIPE $$'`, []string{"PATH=" + dir})
	if st != cannotExecute {
		t.Fatalf("status %d, want the %d that says nothing ran", st, cannotExecute)
	}
	if tries != deathRunAttempts {
		t.Errorf("it gave up after %d attempt(s), want all %d", tries, deathRunAttempts)
	}
	if !strings.Contains(out, "sh") {
		t.Errorf("the shell said %q, which does not name what could not be run", out)
	}
}

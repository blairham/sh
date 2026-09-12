// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A substitution the command never opens must not leave a goroutine behind.
//
// This is the difference between a shell and a library. Opening a named pipe
// waits for the other end, and a command handed a path is under no obligation
// to open it — `echo <(true)` prints a path and is an error in no shell — so
// the shell's own end used to wait forever. A binary exits and takes the
// parked goroutine and its blocked OS thread with it; a program that embeds
// this package accumulates one of each per substitution nobody opened, for as
// long as it runs.
//
// Counted by stack frame rather than by total goroutines, because the total is
// everything else the suite is doing at the same time. That makes the matcher
// the weak point — a renamed function would count nothing and pass — so each
// case first runs a substitution that is *still going* when the command
// returns and requires the count to see it. A matcher that cannot find a
// running one is not evidence about a parked one.
//
// What keeps that one running is a file rather than a sleep, and the
// difference is a failure this had on a loaded machine: a substitution asked
// to sleep for four hundred milliseconds is only still going if the foreground
// beat it to the finish, and on a busy runner under -race it did not. A
// substituted command that waits for a file the test has not written yet
// cannot finish early, whatever the machine is doing.
//
// **The counting happens from outside the run**, and that moved: it used to
// be taken after run() returned, which was only possible while a `>(cmd)`
// could still be going then. It cannot be any more — the command that names
// a writing substitution waits for its body, so that what the body writes
// into the shell's output is there before the shell is done with it (see
// removeProcSubs). A test that wants to see a body in flight has to watch
// from somewhere the shell is not: the script runs on a goroutine of the
// test's, and the release the body is waiting for is written by the watcher
// after it has counted. Nothing else about the measurement changes, and the
// reading direction — which is not waited for — is watched the same way so
// that the three cases stay one shape.
func TestASubstitutionNobodyOpensLeavesNoGoroutineBehind(t *testing.T) {
	for _, tc := range []struct {
		name, unopened string
		// running is a command whose substitution says it has started by
		// creating the file named %[1]s, and then will not finish until the
		// file named %[2]s exists.
		running string
	}{
		// `<(cmd)`: the shell holds the writing end, which is the case the
		// goroutine dumps caught — two of them parked in OpenFile(O_WRONLY),
		// one belonging to a test that had already finished.
		{
			name:     "reading from one",
			unopened: `echo <(true) >/dev/null`,
			// The path is opened and let go of rather than read to the end:
			// `cat` would wait for the substitution to finish, and the count
			// has to be taken while one is still going.
			running: `true < <(` + heldOpenScript + `)`,
		},
		// `>(cmd)`: the shell holds the reading end, and waited for a writer
		// that a command which ignores the path never becomes.
		{
			name:     "writing into one",
			unopened: `echo >(true) >/dev/null`,
			running:  `echo hi > >(` + heldOpenScript + `)`,
		},
		// The same end, with a substituted command that reads rather than
		// one that ignores its input. Nothing is ever going to write to it,
		// so what has to arrive is the end-of-file — and the only thing that
		// can send one is the shell letting go of the pipe it is holding
		// open on the command's behalf.
		{
			name:     "writing into one that reads",
			unopened: `echo >(cat) >/dev/null`,
			running:  `echo hi > >(` + heldOpenScript + `; cat >/dev/null)`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			started := filepath.Join(dir, "started")
			release := filepath.Join(dir, "go-ahead")

			// A quiet baseline rather than whatever the count happens to be.
			// Taking the number as it stands and asking for more than it
			// afterwards was a race of its own: a substitution from the case
			// before, or from another test in this package, can still be
			// finishing when the number is read, and then finish before it
			// is read again — which is a rise and a fall recorded as no rise
			// at all. Nothing here can be relative to a number that moves,
			// so the test waits for the one number that does not.
			if n := waitForGoroutines(0); n != 0 {
				t.Fatalf("%d goroutines already in a substitution, want a quiet start", n)
			}

			src := strings.NewReplacer("%[1]s", started, "%[2]s", release).Replace(tc.running)
			var inFlight int
			st := runWhileWatching(t, src, func() {
				// Counted once the substitution has said it is running, and
				// not merely once the command that named it has started.
				// Those are two different moments and the gap between them
				// is the whole bug this had: the goroutine is started before
				// the substituted command reaches it, so a count taken too
				// early can find the goroutine somewhere on its way in
				// rather than in the frame this counts by. It passed on one
				// platform and failed on the other, which is what a missing
				// synchronization looks like from the outside. The file is
				// written *by the substituted command*, so its existence
				// puts the goroutine inside the run.
				waitForStart(t, started)
				inFlight = substitutionGoroutines()
				if err := os.WriteFile(release, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			})
			if st != 0 {
				t.Fatalf("status %d, want the substitution to run", st)
			}
			if inFlight < 1 {
				t.Fatalf("%d goroutines in a substitution that has said it is running, "+
					"want at least the one — the frame this counts by has moved, "+
					"so the leak below cannot be seen either", inFlight)
			}
			if n := waitForGoroutines(0); n != 0 {
				t.Fatalf("%d goroutines still in a substitution that was let go, want none", n)
			}

			for range 5 {
				if _, st := run(t, tc.unopened, nil); st != 0 {
					t.Fatalf("status %d, want a path nobody opens to be no error", st)
				}
			}
			if n := waitForGoroutines(0); n != 0 {
				t.Errorf("%d goroutines left in a substitution, want none — "+
					"a path nobody opened parked one for good", n)
			}
		})
	}
}

// runWhileWatching runs src on a goroutine of its own and calls watch while it
// is running, answering with the script's status once it has finished.
//
// The watcher runs on the *test's* goroutine, which is what makes it the one
// that may report: everything it does — waiting for the substitution to say it
// has started, counting, releasing it — is allowed to call t.Fatal, and
// nothing in the run is.
func runWhileWatching(t *testing.T, src string, watch func()) int {
	t.Helper()
	f, perr := syntax.Parse(src, syntax.Core())
	if perr != nil {
		t.Fatalf("parse %q: %v", src, perr)
	}
	// The locked writer rather than a bytes.Buffer, because a substitution's
	// body writes to it from its own goroutine while this one is still in the
	// run — which is the whole arrangement under test.
	var buf output
	sem := testSemantics()
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Env: testPATH()})
	var status int
	done := make(chan struct{})
	go func() {
		defer close(done)
		status, _ = r.Run(context.Background(), f)
	}()
	watch()
	<-done
	return status
}

// heldOpenScript is a substituted command that says when it has started and
// then will not finish until the test says so. `%[1]s` is the file it writes
// to say it is running, `%[2]s` the file it waits for.
//
// Both halves are the test's synchronization, and neither can be replaced by a
// number: the first is what puts the goroutine demonstrably inside the frame
// being counted, and the second is what keeps it there for as long as the
// counting takes.
const heldOpenScript = `printf s >%[1]s; while [ ! -e %[2]s ]; do sleep 0.02; done`

// waitForStart blocks until the substituted command has said it is running.
func waitForStart(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s was never written — the substitution never started", path)
		}
		time.Sleep(time.Millisecond)
	}
}

// substitutionGoroutines counts the goroutines a process substitution is
// running on, by the frame every one of them has.
//
// Still the substitution's own frame and not spawn's, now that spawn is what
// starts these. spawn starts all four of the things a shell runs beside itself
// — a background job, either half of a pipeline, a coprocess, a substitution —
// so counting there would count the suite's background jobs as substitutions,
// which is exactly the moving number the quiet start below exists to avoid.
// What the indirection cost was never the name: it was that the goroutine is
// started before the substituted command reaches this frame, and what fixes
// that is waiting for the command to say it is running rather than counting a
// different frame.
//
// The buffer is grown rather than guessed at. runtime.Stack truncates to what
// it is given and says nothing about having done so beyond filling it, and a
// truncated dump counts what it happened to reach — which reads as no
// substitution running at all, and is the same answer this asks for.
func substitutionGoroutines() int {
	for size := 1 << 20; ; size *= 2 {
		buf := make([]byte, size)
		if n := runtime.Stack(buf, true); n < size {
			return strings.Count(string(buf[:n]), "interp.(*Runner).procSub.func")
		}
	}
}

// waitForGoroutines gives the substitutions still finishing a moment to
// finish, so the count is of what is parked rather than of what is in
// flight. Generous, because the answer it is used for is 0 or forever.
func waitForGoroutines(want int) int {
	deadline := time.Now().Add(20 * time.Second)
	for {
		n := substitutionGoroutines()
		if n <= want || time.Now().After(deadline) {
			return n
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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
func TestASubstitutionNobodyOpensLeavesNoGoroutineBehind(t *testing.T) {
	for _, tc := range []struct {
		name, unopened string
		// running is a command with a substitution that will not finish
		// until the file named %s exists.
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
			running: `true < <(` + waitForFileScript + `)`,
		},
		// `>(cmd)`: the shell holds the reading end, and waited for a writer
		// that a command which ignores the path never becomes.
		{
			name:     "writing into one",
			unopened: `echo >(true) >/dev/null`,
			running:  `echo hi > >(` + waitForFileScript + `)`,
		},
		// The same end, with a substituted command that reads rather than
		// one that ignores its input. Nothing is ever going to write to it,
		// so what has to arrive is the end-of-file — and the only thing that
		// can send one is the shell letting go of the pipe it is holding
		// open on the command's behalf.
		{
			name:     "writing into one that reads",
			unopened: `echo >(cat) >/dev/null`,
			running:  `echo hi > >(` + waitForFileScript + `; cat >/dev/null)`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			release := filepath.Join(t.TempDir(), "go-ahead")

			before := substitutionGoroutines()
			if _, st := run(t, strings.ReplaceAll(tc.running, "%s", release), nil); st != 0 {
				t.Fatalf("status %d, want the substitution to run", st)
			}
			if n := substitutionGoroutines(); n <= before {
				t.Fatalf("%d goroutines in a substitution with one still running, "+
					"want more than the %d there were — the frame this counts by is gone, "+
					"so the leak below cannot be seen either", n, before)
			}
			if err := os.WriteFile(release, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			if n := waitForGoroutines(before); n > before {
				t.Fatalf("%d goroutines still in a substitution that was let go, want %d", n, before)
			}

			for range 5 {
				if _, st := run(t, tc.unopened, nil); st != 0 {
					t.Fatalf("status %d, want a path nobody opens to be no error", st)
				}
			}
			if n := waitForGoroutines(before); n > before {
				t.Errorf("%d goroutines left in a substitution, want the %d there were before — "+
					"a path nobody opened parked one for good", n, before)
			}
		})
	}
}

// waitForFileScript is a substituted command that will not finish until the
// test says so. `%s` is the file to wait for.
const waitForFileScript = `while [ ! -e %s ]; do sleep 0.02; done`

// substitutionGoroutines counts the goroutines a process substitution is
// running on, by the frame every one of them has.
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

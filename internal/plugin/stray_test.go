// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package plugin_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/childguard"
	"github.com/blairham/sh/internal/treeguard"
)

// This package launches a process per test and had no guard around the run,
// which is how #1093 got as far as it did: seven orphaned copies of
// testdata/greet, parent pid 1, three hours old, at 11% of a core each. They
// were found by a person looking at `ps`.
//
// The subject splits in two and both halves are here.
//
// A fixture must not be able to spin. `ask` read the host's answer without
// checking the read, so end of input handed back an empty line that the
// `sink` loop could not tell from an answer of "not eof yet" — and it went
// round again on it as fast as it could fork `printf` and `sed`. That is what
// TestAFixtureAbandonedMidCallStopsAskingAndExits and
// TestEveryFixtureExitsWhenItsInputEnds grade, from outside the host, because
// the property belongs to the fixture and not to the host.
//
// And a stray must be reported rather than left for somebody to notice. That
// is TestMain below, and the reason it is not free: see
// TestTheGuardNamesAFixtureThisRunLeftRunning.

// strayMarker is the string a command line has to contain for the process to
// be one of this package's fixtures.
//
// The path is the marker, which is what makes it available at all: a fixture
// is a shell script started by its absolute path, so `ps` shows
// `/bin/sh …/internal/plugin/testdata/greet` and the directory names itself.
// Owned here rather than in childguard because it is *this* package's
// testdata, where the pipe prefix is the interpreter's; and pinned to a real
// fixture path by TestTheGuardLooksForTheNameTheseFixturesAreAt, so a moved
// directory fails the build rather than leaving the guard finding nothing.
//
// Slash-separated rather than filepath.Join because it has to be a const and
// because every route into this program is a POSIX one.
const strayMarker = "internal/plugin/testdata"

// TestMain guards the run.
//
// Both guards, and the childguard one with this package's own marker rather
// than childguard.PipeMarker. Nothing here holds a process-substitution pipe,
// so the pipe marker would have matched nothing and reported nothing —
// forever and silently, which is the one thing a guard may not do.
//
// It cannot be the whole answer to #1093 and that is worth writing down. The
// census is taken after the run returns, so a binary that is *killed* — a
// `go test` timeout, a SIGKILL — never reaches it, and a killed run is
// precisely how those seven were orphaned. A guard reports the ordinary leak;
// what stops the killed run leaving a process per launch behind is the
// fixture exiting on its own end of input, which is the other half above.
func TestMain(m *testing.M) {
	os.Exit(treeguard.Run(childguard.Wrap(m, strayMarker)))
}

// abandonBound is how long a fixture whose host has gone is given to work out
// that it has nothing left to do.
//
// A fixture that reads its input correctly is gone in milliseconds; this is
// the bound a *spinning* one has to sit out before it is called a spin, so it
// is long enough not to fire on a loaded machine and short enough that a run
// which really did regress is not made to wait twice for the news.
const abandonBound = 10 * time.Second

// deliberatelyDeaf are the fixtures that must NOT exit at end of input,
// because being unreachable is the property each of them is for. This is the
// hazard childguard's own doc warns about from the other side — a guard that
// fires on what was the point gets turned off within the day — so the
// exemption is a named reason each rather than a list that grows.
//
// Both are taken away by the host's process group, which is what their own
// tests grade. And deafobserver is worth one more note: it replaces itself
// with `sleep 300`, so its command line stops naming testdata and the
// marker above cannot see it either. That is deliberate on its part — one
// process to kill, spawning nothing — and it means these two are covered by
// Close and by nothing else here.
var deliberatelyDeaf = map[string]string{
	"hang":         "never answers the handshake at all; the host giving up on a bound is what its test grades",
	"deafobserver": "handshakes and then stops reading, so the host's feed blocks in a write and Close is the only way out — the lifetime rule's worked example",
}

const (
	// The handshake, and one call that asks the host for something and is
	// then never answered. `sink` is the only command in these fixtures that
	// loops on a host method, so it is the one that could spin.
	initializeRequest = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	invokeSinkRequest = `{"jsonrpc":"2.0","id":2,"method":"command/invoke","params":{"call":"c1","name":"sink","args":["x"]}}`
)

// abandon runs a fixture with requests on its standard input and nothing
// after them, which is the state a plugin is left in when its host goes away:
// end of input, with a call outstanding and an unanswered request on the
// wire. It answers what the fixture wrote and whether it exited on its own.
func abandon(t *testing.T, name string, requests ...string) (out string, exited bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), abandonBound)
	defer cancel()
	cmd := exec.CommandContext(ctx, fixture(t, name))
	cmd.Stdin = strings.NewReader(strings.Join(requests, "\n") + "\n")
	// Drained, and not into a pipe nothing reads. A plugin whose output pipe
	// has filled up is *blocked*, so a discarded stream would stop a spin and
	// then report that there was none — the test would pass on the bug.
	wrote := &syncBuffer{}
	cmd.Stdout = wrote
	cmd.Stderr = &syncBuffer{}
	// Nothing this test starts may outlive it, since that is the subject.
	// Cancel kills the process; WaitDelay is what stops the wait blocking on
	// a descendant that inherited the write end of a stream.
	cmd.WaitDelay = time.Second
	// A fixture may exit non-zero on purpose, so the status is not the
	// question here — whether it exited at all is.
	_ = cmd.Run()
	return wrote.String(), ctx.Err() == nil
}

// TestAFixtureAbandonedMidCallStopsAskingAndExits is #1093.
//
// Two assertions and they fail for different reasons. Exiting is the property
// that stops a killed test run leaving a process behind. Asking exactly once
// is the sharper one: a fixture could stop spinning by *blocking* instead,
// which would leave a process behind just as surely and would satisfy nothing
// but a count.
func TestAFixtureAbandonedMidCallStopsAskingAndExits(t *testing.T) {
	out, exited := abandon(t, "greet", initializeRequest, invokeSinkRequest)
	if !exited {
		t.Errorf("still running after %v with its host gone; a plugin at end of input has nothing left to do.\nit wrote %d input requests", abandonBound, strings.Count(out, `"method":"shell/read"`))
	}
	if n := strings.Count(out, `"method":"shell/read"`); n != 1 {
		t.Errorf("asked the host for input %d times, want exactly 1: a request nothing answered because the host is gone is not an answer of \"not eof yet\"", n)
	}
}

// TestEveryFixtureExitsWhenItsInputEnds is the class rather than the case.
//
// Correcting the one loop that spun does not stop the next one, which is the
// argument every guard in this tree rests on. End of input on a plugin's
// standard input is unambiguous — Host.Close closes it first, precisely so a
// well-behaved plugin sees it and goes — so every one of these has to.
func TestEveryFixtureExitsWhenItsInputEnds(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "*"))
	if err != nil {
		t.Fatalf("listing the fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no fixtures found; this test would pass by grading nothing")
	}
	for _, path := range paths {
		name := filepath.Base(path)
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			continue
		}
		if why, deaf := deliberatelyDeaf[name]; deaf {
			t.Logf("%s: not graded — %s", name, why)
			continue
		}
		t.Run(name, func(t *testing.T) {
			if _, exited := abandon(t, name, initializeRequest); !exited {
				t.Errorf("still running %v after its input ended", abandonBound)
			}
		})
	}
}

// TestTheGuardLooksForTheNameTheseFixturesAreAt pins the two together.
//
// The same reason interp pins its pipe prefix to childguard.PipeMarker: a
// directory that moved in one place and not the other leaves the guard
// matching nothing, and a guard that finds nothing is indistinguishable from
// a clean run.
func TestTheGuardLooksForTheNameTheseFixturesAreAt(t *testing.T) {
	if path := fixture(t, "greet"); !strings.Contains(path, strayMarker) {
		t.Errorf("the fixtures are at %q and the guard looks for %q; a fixture's own path is what makes it findable", path, strayMarker)
	}
}

// TestTheGuardNamesAFixtureThisRunLeftRunning hands the detector a real one.
//
// This is the point of the whole file. A detector that reports nothing passes
// every run and looks exactly like a clean tree, so being told "no strays" is
// worth nothing until something checks that it can still say the opposite.
// The violation is real rather than a table of invented rows: a fixture this
// test binary started, blocked on an input nothing is writing to, which is
// the state every leaked plugin was found in.
func TestTheGuardNamesAFixtureThisRunLeftRunning(t *testing.T) {
	if found, err := childguard.Holding(os.Getpid(), strayMarker); err != nil {
		t.Skipf("no process table here: %v", err)
	} else if len(found) != 0 {
		t.Fatalf("a fixture was already running before this test started it: %v", found)
	}
	// Killed by the context when the test ends, and the stdin pipe is closed
	// first so it goes the way a plugin is meant to: on its own end of input,
	// with no signal needed and nothing of its own left behind.
	cmd := exec.CommandContext(t.Context(), fixture(t, "greet"))
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("opening the fixture's input: %v", err)
	}
	cmd.Stdout = &syncBuffer{}
	cmd.Stderr = &syncBuffer{}
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting the fixture: %v", err)
	}
	t.Cleanup(func() {
		_ = in.Close()
		_ = cmd.Wait()
	})

	// Started is not yet listed by `ps`, and the guard reports on the process
	// table rather than on what this test knows it did.
	deadline := time.Now().Add(abandonBound)
	for {
		found, err := childguard.Holding(os.Getpid(), strayMarker)
		if err != nil {
			t.Skipf("no process table here: %v", err)
		}
		for _, p := range found {
			if p.PID == cmd.Process.Pid {
				return
			}
		}
		if !time.Now().Before(deadline) {
			t.Fatalf("the guard reported nothing for a fixture this run left running (pid %d); silence and a clean tree are the same report, which is what makes this worth checking", cmd.Process.Pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

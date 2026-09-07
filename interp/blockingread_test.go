// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A background job whose body *reads* is the one shape that could leave the
// shell that started it waiting forever, and a coprocess reaches it every
// time: its input is a pipe the shell holds the write end of, so a `read` in
// its body waits by construction (#1277).
//
// Every case here is bounded. The bug is a shell that never returns, and a
// test that simply called Run would hang the suite rather than fail it —
// which is a worse outcome than the bug, because a hung CI job reports
// nothing at all.

// blockingReadBound is how long a shell gets before the test calls it a hang.
//
// Generous rather than tight, and the asymmetry is the reason: the answer
// arrives in microseconds when the latch is there, so a longer bound costs
// only the length of a *failure*, while a shorter one buys a flake on a
// machine running several of these suites at once.
const blockingReadBound = 30 * time.Second

// TestACoprocessWhoseBodyReadsDoesNotBlockTheShell is the reported bug.
//
// `coproc read x` printed nothing here and never ended; zsh 5.9.2, bash
// 5.3.15 and ksh93u+ 2012-08-01 all print `AFTER` at once, having forked
// before the body runs a thing.
func TestACoprocessWhoseBodyReadsDoesNotBlockTheShell(t *testing.T) {
	for _, src := range []string{
		"coproc read -r l\necho AFTER",
		"coproc { read -r l; }\necho AFTER",
		"coproc { read -r l; echo got; }\necho AFTER",
		// The letters that decorate the same wait. A deadline bounds the
		// second one and does not remove it, so the shell that started the
		// job was blocked for the whole of it rather than only until the pid
		// was known — and the deadline is deliberately longer than this
		// file's bound, so that a shell which waits it out fails the test
		// instead of passing it slowly.
		"coproc read -r -d : l\necho AFTER",
		"coproc read -t 60 -r l\necho AFTER",
	} {
		t.Run(src, func(t *testing.T) {
			out, st := runBoundedScript(t, src, coprocGrammar, nil)
			if out != "AFTER\n" || st != 0 {
				t.Errorf("got %q status %d, want the shell to carry on past the coprocess", out, st)
			}
		})
	}
}

// TestACoprocessWhoseBodyMapfilesDoesNotBlockTheShell is the second builtin
// that waits on a stream, and it is here because a fix in `read` alone would
// leave it hanging.
func TestACoprocessWhoseBodyMapfilesDoesNotBlockTheShell(t *testing.T) {
	for _, src := range []string{
		"coproc mapfile arr\necho AFTER",
		"coproc readarray arr\necho AFTER",
	} {
		t.Run(src, func(t *testing.T) {
			out, st := runBoundedScript(t, src, coprocGrammar, nil)
			if out != "AFTER\n" || st != 0 {
				t.Errorf("got %q status %d, want the shell to carry on past the coprocess", out, st)
			}
		})
	}
}

// TestACoprocessRunningSelectDoesNotBlockTheShell is the third, and the one
// that reads through a *clause* rather than a builtin: `select` draws its menu
// and then waits for a choice on a stream nobody is writing.
//
// The menu comes out before it, and on the shell's own error stream rather
// than down the pipe — a coprocess redirects the two named streams and leaves
// complaints where whoever is watching the shell can read them. So the
// assertion is on what the shell reached, not on the whole of the output.
func TestACoprocessRunningSelectDoesNotBlockTheShell(t *testing.T) {
	src := "coproc { select v in a; do break; done; }\necho AFTER"
	out, st := runBoundedScript(t, src, func(d *syntax.Dialect) {
		coprocGrammar(d)
		d.Select = true
	}, nil)
	if !strings.HasSuffix(out, "AFTER\n") || st != 0 {
		t.Errorf("got %q status %d, want the shell to carry on past the coprocess", out, st)
	}
}

// TestABackgroundJobBlockedOnItsInputDoesNotBlockTheShell is the same latch
// reached by `&` rather than by a coprocess.
//
// The stream is what makes the difference here, which is why the pipe is built
// rather than borrowed: `{ read x; } &` under `/dev/null` finishes at once and
// never reaches the wait at all. A pipe with a writer and no data is the shape
// that waits, and it is the one a shell reading a fifo has.
func TestABackgroundJobBlockedOnItsInputDoesNotBlockTheShell(t *testing.T) {
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	// Closed however the test ends, which is also what releases the job: the
	// read it is standing in returns at end of input.
	t.Cleanup(func() {
		_ = pw.Close()
		_ = pr.Close()
	})
	out, st := runBoundedScript(t, "{ read -r l; } &\necho AFTER", nil, func(r *Runner) {
		r.Stdin = pr
	})
	if out != "AFTER\n" || st != 0 {
		t.Errorf("got %q status %d, want the shell to carry on past the background job", out, st)
	}
}

// TestABackgroundJobReadingAReadyStreamStillReportsItsProcess is the other
// side of the latch, and the reason it asks the stream rather than settling on
// every read.
//
// A regular file answers a read at once, so the job is left alone and `$!` is
// still the process it goes on to start. Settling unconditionally would print
// 0 here, which is a real answer lost — the same trade the blocking-open latch
// states for `sleep 0.3 > log & echo $!`.
func TestABackgroundJobReadingAReadyStreamStillReportsItsProcess(t *testing.T) {
	var dir string
	out, st := runBoundedScript(t,
		"printf 'x\\n' > f\n{ read -r l < f; /bin/echo ran; } &\ncase $! in 0) echo nopid;; *) echo pid;; esac\nwait",
		nil, func(r *Runner) { dir = r.Dir })
	if dir == "" {
		t.Fatal("the helper left Dir empty")
	}
	if out != "ran\npid\n" && out != "pid\nran\n" {
		t.Errorf("got %q status %d, want the job's own process reported through $!", out, st)
	}
}

// coprocGrammar turns on the keyword and the name that may follow it.
func coprocGrammar(d *syntax.Dialect) {
	d.Coproc = true
	d.CoprocName = true
}

// runBoundedScript runs src and fails rather than waiting when it does not
// return.
//
// The run is on a goroutine of the test's own so the deadline can be watched
// from the test's, which is the only arrangement where a hang is a failure
// with a message on it. A run that overruns is left where it stands: it is
// blocked on something that is not coming, and the binary is about to end.
func runBoundedScript(t *testing.T, src string, enable func(*syntax.Dialect), setup func(*Runner)) (string, int) {
	t.Helper()
	d := syntax.Core()
	if enable != nil {
		enable(&d)
	}
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf output
	sem := testSemantics()
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Env: testPATH(),
	})
	if setup != nil {
		setup(r)
	}
	type answer struct {
		status int
		err    error
	}
	done := make(chan answer, 1)
	go func() {
		st, err := r.Run(context.Background(), f)
		done <- answer{st, err}
	}()
	select {
	case a := <-done:
		if a.err != nil {
			t.Fatalf("run %q: %v", src, a.err)
		}
		return buf.String(), a.status
	case <-time.After(blockingReadBound):
		t.Fatalf("the shell did not return within %s: %q", blockingReadBound, src)
		return "", 0
	}
}

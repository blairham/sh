// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// callersWriter is an io.Writer with no synchronization of its own, which is
// what an io.Writer is.
//
// The unguarded field is the assertion and not an oversight: a caller handing
// a shell one of these has promised nothing about concurrent use, so if two of
// the shell's writers reach it at once the race detector says so. Everything
// the test itself reads is behind the mutex, so the only way to make this
// report anything is for the shell to write from two places without taking the
// lock it puts over the stream.
type callersWriter struct {
	mu sync.Mutex
	// text is read by the test and written under the lock.
	text strings.Builder
	// races is deliberately not: it is the tripwire.
	races int
}

func (w *callersWriter) Write(p []byte) (int, error) {
	w.races++
	w.mu.Lock()
	defer w.mu.Unlock()
	w.text.Write(p)
	return len(p), nil
}

func (w *callersWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.text.String()
}

// A child's stream reaches the caller's writer through the lock the shell puts
// over it.
//
// #735. interp serializes writes to a caller's io.Writer with a lock of its
// own, because the shell is what created the concurrency and an io.Writer
// carries no promise of being safe to write from two places. A **child
// process** did not go through it: when a command's Stderr is not an *os.File,
// os/exec makes a pipe and copies into the writer on a goroutine of its own,
// and that goroutine held no share of the lock.
//
// `cat <(cmd)` is the shape, because a process substitution wrapped only its
// own side and left the shell writing raw — the arrangement lockedWriter's own
// comment rules out, arrived at from the other side. bytes.Buffer.ReadFrom
// grows the buffer before it reads, so a child that writes *nothing* still
// races.
//
// # Why every round is bounded
//
// This case has a second failure mode that is not a failure: under scheduling
// pressure the first subtest *stops* rather than answering, and an unbounded
// run then reports it as `panic: test timed out after 10m0s` with the package
// name and nothing else (#1066). The bound turns that into a named subtest, a
// round number and a sentence, in twenty seconds. It is not there to let a
// slow run pass — a healthy round is milliseconds.
//
// What the stop actually was, measured rather than assumed, because the
// issue's first reading was that the goroutine draining the child was blocked
// on a write and the lock was involved. It was neither. Reproduced at
// `GOMAXPROCS=1`, with a goroutine dump and `lsof` and `sample` on the child:
// the shell in `wait4`, both drains in `poll.Read`, the substitution's
// producer goroutine finished, and `cat` asleep in `read` on a pipe with no
// writer at all — waiting for an end-of-file that had already been and gone.
//
// That was a defect in the shell rather than in this test — `cat <(cmd)` can
// hang a script — and it was #1079, fixed in interp.nudgeFifoEOF, which
// carries the mechanism and the numbers. This test's job was to stop hiding
// it, and the case that watches for its return is
// TestAProcessSubstitutionAlwaysDeliversItsEndOfFile.
func TestAChildsStreamTakesTheLockTheShellPutsOverACallersWriter(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a process substitution beside a child", `cat <(echo sub; echo noise >&2)`},
		{
			"an output substitution beside a child",
			`/bin/sh -c 'echo out; echo err >&2' > >(cat >/dev/null; echo noise >&2)`,
		},
		{
			"a coprocess beside a child",
			`coproc { sleep 0.02; echo noise >&2; }; /bin/sh -c 'sleep 0.05; echo err >&2'`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := syntax.Core()
			d.Coproc = true
			f, err := syntax.Parse(tc.src, d)
			if err != nil {
				t.Fatal(err)
			}
			for round := range 20 {
				w := &callersWriter{}
				sem := testSemantics()
				dg := PosixDiagnostics()
				r := newTestRunner(t, &Runner{
					Stdout: w, Stderr: w, Semantics: &sem, Diagnostics: &dg, Env: testPATH(),
				})
				// Bounded, one round at a time, because this case does not
				// fail when it goes wrong — it stops (#1066). See below.
				deadline(t, fmt.Sprintf("%s, round %d of 20", tc.name, round), func() {
					if _, err := r.Run(context.Background(), f); err != nil {
						t.Error(err)
						return
					}
					settle(r)
				})
			}
		})
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// An interpreter bug on one of the shell's own goroutines.
//
// A shell runs four things beside itself and every one is a goroutine here
// where a real shell forks — a background job, either half of a pipeline, a
// coprocess, a process substitution. `recover` reaches only the goroutine that
// panicked, so the guard a front end puts around a run cannot see any of them,
// and a bug hit on one used to end the process however carefully the caller had
// wrapped its call.
//
// Two halves, and the tests here are about the second. A front end supplies the
// recover through GuardConcurrent; what this package still owes is that
// everything the rest of the shell is waiting on is handed over on the way out.
// Each case below is written so that a shell with the recover and without the
// handover **hangs**: `wait` never returns for a job that never finished, a
// pipeline never finishes for an element that never closed its pipe, and a
// coprocess is read until an end-of-file that never comes. That is the failure
// worth guarding against, because it is worse than the crash it replaced.
func TestABugOnTheShellsOwnGoroutineCostsOnlyThatGoroutine(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		enable          func(*syntax.Dialect)
	}{
		// `wait` is the whole test: it returns only for a job that has
		// finished. Named rather than bare, because a bare `wait` reports 0
		// whatever the jobs did and would say nothing about the status.
		{
			name: "a background job",
			src:  `boom & wait -n; echo "after=$?"`,
			want: "after=2\n",
		},
		// The element downstream reads until its input is closed, and the
		// pipeline is not over until every element is. `cat` producing
		// nothing and the line after it running is the pipe having been
		// closed by an element that never got to close it itself.
		{
			name: "a pipeline element",
			src:  `boom | cat; echo "after=$?"`,
			want: "after=0\n",
		},
		// The status of an element that stopped without finishing, which is
		// only visible where a pipeline reports something other than its
		// last element. The zero value of that slice is 0, so an unset one
		// reads as a success.
		{
			name: "a pipeline element, with pipefail",
			src:  `set -o pipefail; boom | cat 2>/dev/null; echo "after=$?"`,
			want: "after=2\n",
		},
		// Reading what the coprocess wrote: it wrote nothing, and the read
		// ends because the shell closed the command's ends on its way out.
		// Without that this line waits forever.
		{
			name:   "a coprocess",
			src:    `coproc boom; read -r line <&"${COPROC[0]}"; echo "after=$?"`,
			want:   "after=1\n",
			enable: func(d *syntax.Dialect) { d.Coproc = true },
		},
		// And the substitution's end of the pipe, which is the end-of-file
		// `cat` is waiting for.
		{
			name: "a process substitution",
			src:  `read -r line < <(boom); echo after`,
			want: "after\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := syntax.Core()
			if tc.enable != nil {
				tc.enable(&d)
			}
			f, err := syntax.Parse(tc.src, d)
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			// A stream of its own rather than the shared helper's, and
			// synchronized, because these four constructs outlive the
			// statement that started them: a report about one can land
			// after the run has returned, and reading a plain bytes.Buffer
			// here would be the caller reading its own writer with no lock
			// on it.
			var out syncBuffer
			var g concurrentGuard
			bash := testSemantics()
			r := newTestRunner(t, &Runner{
				Stdout: &out, Stderr: &out, Semantics: &bash, Env: testPATH(),
			})
			r.GuardConcurrent = g.run
			r.Register("boom", func(*Runner, context.Context, []string) int {
				panic("an invariant broke")
			})
			// On a goroutine with a deadline, because the failure each of
			// these is written to catch is a shell that never comes back.
			// Left to the package's own timeout it would be ten minutes and
			// a goroutine dump naming whichever test was unlucky enough to
			// be running; and a shell can be rescued from it by chance,
			// since Go closes a file its finalizer reaches — which turned
			// one of these hangs into a two-and-a-half-minute pause and a
			// pass.
			type done struct {
				st  int
				err error
			}
			finished := make(chan done, 1)
			go func() {
				st, err := r.Run(context.Background(), f)
				finished <- done{st, err}
			}()
			var res done
			select {
			case res = <-finished:
			case <-time.After(30 * time.Second):
				t.Fatal("the shell never came back — something it was waiting " +
					"on was never handed over")
			}
			st, rerr := res.st, res.err
			if rerr != nil {
				t.Fatalf("run: %v", rerr)
			}
			if st != 0 {
				t.Errorf("status %d, want the shell itself to have finished", st)
			}
			if got := out.String(); got != tc.want {
				t.Errorf("out = %q, want %q", got, tc.want)
			}
			if n := g.caught(); n != 1 {
				t.Errorf("the guard caught %d panics, want the one that was raised", n)
			}
		})
	}
}

// concurrentGuard is a front end's answer to a panic on one of the shell's own
// goroutines, in the smallest form that is still one: recover, count, and go
// on. What a real one adds is the diagnostic.
type concurrentGuard struct {
	mu sync.Mutex
	n  int
}

func (g *concurrentGuard) run(work func(), _ io.Writer) {
	defer func() {
		if v := recover(); v != nil {
			g.mu.Lock()
			g.n++
			g.mu.Unlock()
		}
	}()
	work()
}

func (g *concurrentGuard) caught() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.n
}

// syncBuffer is a caller's stream that is safe to read while the shell it was
// handed to may still be writing to it.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

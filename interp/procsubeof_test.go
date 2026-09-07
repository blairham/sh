// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp_test

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `cat <(cmd)` finishes, every time.
//
// The regression case for #1079, and it is a stress case because the defect is
// a race and there is no other honest shape for it. The mechanism, the
// evidence and the standalone reproduction are in interp.nudgeFifoEOF.
//
// # What it detects, measured on the unfixed tree
//
// macOS 26.5.2 on arm64. Three runs of the case as written: two of them
// stopped — `cat <(true)` at round 196, `cat <(echo sub; echo noise >&2)` at
// rounds 200 and 567 — and one got through both shapes. So it is a *rate*
// rather than a certainty, which is what a race deserves and what the number
// of rounds is chosen against: two thousand of each is about ten seconds and
// four rounds in five.
//
// # Why the loop is this tight
//
// The round has to be a few milliseconds, and the first version of this case
// was not: `t.TempDir()` twice per round, a helper goroutine and a fresh
// timeout context took it to seventeen milliseconds, and at that pace the race
// **stopped appearing at all** — three runs of three thousand rounds on the
// unfixed tree without one stop, against roughly one in five hundred from a
// four-millisecond loop. So the directory, the environment and the output
// buffer are made once per shape, each round's shell is told to clean up after
// itself, and the bound is a timer this reuses rather than a helper.
//
// That is worth knowing beyond this case: a stress case for a scheduling race
// can be slowed into uselessness by its own scaffolding, and it will still be
// green.
//
// # Why GOMAXPROCS is pinned
//
// At full parallelism on a quiet machine the race does not appear, and the
// case would be watching nothing. Pinning is a change to the whole process, so
// it is done here and nowhere else, and it is put back; the tests that run
// beside it are only made slower.
//
// # Why the bound can end the round
//
// `cancel()` and then `<-done` returns, which before #1075 it would not have:
// the runner consults its context now, so a round that would have hung is
// stopped and the case reports a round number instead of a package timeout.
func TestAProcessSubstitutionAlwaysDeliversItsEndOfFile(t *testing.T) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))

	for _, tc := range []struct{ name, src string }{
		// The substitution that writes nothing at all is the sharpest: the
		// window between the shell's write end opening and closing again is
		// at its narrowest, so the reader's own open is at its most likely
		// to still be in progress when the end-of-file goes past.
		{"a substitution that writes nothing", `cat <(true)`},
		{"a substitution that writes and complains", `cat <(echo sub; echo noise >&2)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := syntax.Core()
			d.ProcessSubstitution = true
			f, err := syntax.Parse(tc.src, d)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			env := append(testPATH(), "TMPDIR="+dir)
			var out lockedOutput
			const rounds = 2000
			for round := range rounds {
				sem := testSemantics()
				r := newTestRunner(t, &Runner{
					Stdout: &out, Stderr: &out, Semantics: &sem, Env: env, Dir: dir,
				})
				ctx, cancel := context.WithCancel(context.Background())
				done := make(chan struct{})
				go func() {
					defer close(done)
					_, _ = r.Run(ctx, f)
				}()
				timer := time.NewTimer(8 * time.Second)
				select {
				case <-done:
					timer.Stop()
				case <-timer.C:
					cancel()
					<-done
					r.CleanUp()
					t.Fatalf("%s, round %d of %d: never saw its end-of-file", tc.name, round, rounds)
				}
				cancel()
				// Per round rather than through the helper's t.Cleanup: two
				// thousand pipe directories left standing until the end of
				// the case is a slower loop, and a slower loop is one that
				// watches for nothing. See above.
				r.CleanUp()
			}
		})
	}
}

// lockedOutput is a writer the shell may hand to two goroutines at once, which
// is what a process substitution beside a child does — see childlock_test.go
// for the case that is about exactly that. strings.Builder is not one, and the
// race detector would say so.
type lockedOutput struct {
	mu sync.Mutex
	b  strings.Builder
}

func (o *lockedOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.b.Write(p)
}

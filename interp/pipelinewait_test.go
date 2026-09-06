// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// settledWriter is a caller's stream that remembers whether anything was
// written to it after the run was over.
//
// The lock is not decoration and is the whole point of the type: the question
// is whether a goroutine the run no longer waits for is still writing, so
// asking has to be safe to do while one might be.
type settledWriter struct {
	mu       sync.Mutex
	text     strings.Builder
	returned bool
	late     int
}

func (w *settledWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.returned {
		w.late += len(p)
	}
	w.text.Write(p)
	return len(p), nil
}

// settled marks the run as over and reports everything written up to here.
func (w *settledWriter) settled() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.returned = true
	return w.text.String()
}

func (w *settledWriter) lateBytes() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.late
}

// A pipeline's writes have all arrived by the time Run returns.
//
// #732 asked this as a real question rather than a rhetorical one. The race it
// reported was the test helper's — a plain bytes.Buffer read while a pipeline
// element was still writing it — and the helper now guards the buffer. But the
// issue offered a second candidate and said it was worth measuring before
// assuming it was only the test: *if a pipeline can return while a branch is
// still writing, then a front end that closes its writer when the run returns
// can lose a diagnostic.*
//
// This is that measurement, kept. runPipeline waits on every element it
// spawned, so the answer is no — and the shape that would show otherwise is
// the one asserted here: an element that takes fifty milliseconds to say
// anything, in a pipeline whose other element ends at once. A pipeline that
// returned when its *last* element finished, or that stopped waiting for a
// branch on any other ground, would leave that line unwritten at the moment
// Run came back, every time rather than sometimes.
//
// What is *not* the same claim, and is why this test is about pipelines: a
// background job does keep writing afterwards, deterministically, and that is
// what `&` means in every shell. Measured for both: 25 of 25 runs of
// `{ sleep 0.05; echo late >&2; } &` wrote after Run returned, and 0 of 200
// runs of every pipeline shape below did.
func TestAPipelinesWritesHaveAllArrivedWhenRunReturns(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			name: "a slow element upstream",
			src:  "{ sleep 0.05; echo slow >&2; } | cat",
			want: "slow\n",
		},
		{
			name: "a slow element last",
			src:  "cat </dev/null | { sleep 0.05; echo slow >&2; }",
			want: "slow\n",
		},
		{
			name: "both elements writing",
			src:  "{ sleep 0.05; echo up >&2; } | { sleep 0.02; echo down >&2; }",
			want: "down\nup\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			w := &settledWriter{}
			sem := testSemantics()
			dg := PosixDiagnostics()
			r := newTestRunner(t, &Runner{
				Stdout: w, Stderr: w, Semantics: &sem, Diagnostics: &dg, Env: testPATH(),
			})
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			got := w.settled()
			// Compared as a set of lines, because two elements run at once
			// and which of them reaches the stream first is not the claim.
			// That everything they wrote is *there* is.
			if !sameLines(got, tc.want) {
				t.Fatalf("Run returned with %q written, want %q — an element was still"+
					" writing when the pipeline said it was done", got, tc.want)
			}
			if n := w.lateBytes(); n != 0 {
				t.Errorf("%d byte(s) arrived after Run returned", n)
			}
		})
	}
}

// sameLines reports whether two texts hold the same lines in any order.
func sameLines(a, b string) bool {
	as := strings.Split(strings.TrimSuffix(a, "\n"), "\n")
	bs := strings.Split(strings.TrimSuffix(b, "\n"), "\n")
	slices.Sort(as)
	slices.Sort(bs)
	return slices.Equal(as, bs)
}

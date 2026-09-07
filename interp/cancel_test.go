// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runCancelable runs src under a context the caller controls, and reports
// what the run came back with. The whole point of the file: before #1075 none
// of these returned at all.
func runCancelable(t *testing.T, ctx context.Context, src string, enable func(*syntax.Dialect)) (out string, status int, err error) {
	t.Helper()
	d := syntax.Core()
	if enable != nil {
		enable(&d)
	}
	f, perr := syntax.Parse(src, d)
	if perr != nil {
		t.Fatalf("parse %q: %v", src, perr)
	}
	var buf strings.Builder
	sem := testSemantics()
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Env: testPATH()})
	done := make(chan struct{})
	go func() {
		defer close(done)
		status, err = r.Run(ctx, f)
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatalf("the run did not stop: the context was not honored")
	}
	return buf.String(), status, err
}

// A runaway script stops when its caller says so.
//
// Four shapes, and the arithmetic loop is the one that matters: it has no
// guard of its own and no end of its own, so nothing but this can stop it —
// which is why #894's test could only be bounded and not ended.
func TestARunawayScriptStopsWhenTheCallerCancelsIt(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		enable    func(*syntax.Dialect)
	}{
		{"an endless arithmetic loop", `for ((;;)); do :; done`, func(d *syntax.Dialect) { d.CStyleFor = true }},
		{"an endless while", `while :; do :; done`, nil},
		{"an endless until", `until false; do :; done`, nil},
		{"an endless loop in a function", `f() { while :; do :; done; }; f`, nil},
		{"an endless loop in a subshell", `( while :; do :; done )`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			_, st, err := runCancelable(t, ctx, tc.src, tc.enable)
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("err = %v, want context.DeadlineExceeded", err)
			}
			if st == 0 {
				t.Errorf("status = 0, want the number a shell stopped from outside reports")
			}
		})
	}
}

// A cancellation cannot be caught by a boundary that catches errors.
//
// The half that keeps this from being a shell nobody can stop. `.`, `eval`, a
// startup file and an interactive prompt all give up one *file* or one *line*
// over an error and carry on; none of them may swallow a caller's request to
// stop, which is why the unwinding carries abandonRequested rather than
// abandonError.
func TestACanceledRunIsNotCaughtByAGiveUpBoundary(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	out, _, err := runCancelable(t, ctx,
		`eval 'while :; do :; done'; echo after-eval`, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if strings.Contains(out, "after-eval") {
		t.Errorf("out = %q, want the run stopped rather than the eval given up", out)
	}
}

// A run nobody cancels is unaffected, which is the property the check has to
// keep: it fires on a closed channel and on nothing else.
func TestAnUncanceledRunFinishesNormally(t *testing.T) {
	out, st, err := runCancelable(t, context.Background(), `for i in 1 2 3; do printf "[%s]" "$i"; done`, nil)
	if err != nil || st != 0 || out != "[1][2][3]" {
		t.Errorf("out = %q status = %d err = %v, want [1][2][3] 0 <nil>", out, st, err)
	}
	// And a cancelable context that is never canceled is the same run.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out, st, err = runCancelable(t, ctx, `for i in 1 2 3; do printf "[%s]" "$i"; done`, nil)
	if err != nil || st != 0 || out != "[1][2][3]" {
		t.Errorf("with a live context: out = %q status = %d err = %v", out, st, err)
	}
}

// A context already canceled when the run starts stops it before the first
// command, rather than letting one through.
func TestARunStartedUnderACanceledContextRunsNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out, _, err := runCancelable(t, ctx, `echo one; echo two`, nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if out != "" {
		t.Errorf("out = %q, want nothing to have run", out)
	}
}

// The status is set once, by the cancellation, and not rewritten by each
// command that then does not run.
func TestACanceledRunKeepsTheStatusTheCancellationSet(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, first, _ := runCancelable(t, ctx, `echo one`, nil)
	_, many, _ := runCancelable(t, ctx, `echo one; echo two; echo three; echo four`, nil)
	if first != many {
		t.Errorf("one command reported %d and four reported %d, want the same", first, many)
	}
}

// What the check costs, on the loop that pays for it most: 200,000 rounds of
// a builtin, which is the shape a runaway script has and the one the door is
// on. Two contexts, because they are two different costs — a nil channel is a
// comparison, a live one is also a select that takes its default arm.
//
// Run as `go test ./interp/ -run XXX -bench Cancel`. The figures this was
// decided on are in the commit that added the file; what the benchmark is for
// is the next person, who can put the check back behind a flag and re-measure
// rather than take the number on trust.
func BenchmarkCancelCheck(b *testing.B) {
	d := syntax.Core()
	d.CStyleFor = true
	f, err := syntax.Parse(`for ((i=0;i<200000;i++)); do :; done`, d)
	if err != nil {
		b.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		ctx  func(b *testing.B) context.Context
	}{
		{"no channel to watch", func(*testing.B) context.Context { return context.Background() }},
		{"a live cancelable context", func(b *testing.B) context.Context {
			ctx, cancel := context.WithCancel(context.Background())
			b.Cleanup(cancel)
			return ctx
		}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			ctx := tc.ctx(b)
			sem := testSemantics()
			for b.Loop() {
				var out strings.Builder
				// testrunner:bare — a benchmark of the per-command check,
				// where the helper's directory, TMPDIR and cleanup would be
				// the thing being timed. Nothing here opens a file.
				r := &Runner{Stdout: &out, Stderr: &out, Semantics: &sem}
				if _, err := r.Run(ctx, f); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// The unit of a cancellation is the chunk the context was given to.
//
// A front end feeding a Runner one typed line at a time hands each line a
// context of its own, and a line whose context was canceled costs that line
// rather than the session: the variables and functions of the chunks before it
// are still there, and the next chunk runs. driver's own session test pins the
// same rule from the outside; this is it at the library's own boundary.
func TestACanceledChunkLeavesTheShellUsable(t *testing.T) {
	sem := testSemantics()
	var out strings.Builder
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out, Semantics: &sem, Env: testPATH()})
	part := func(ctx context.Context, src string) error {
		f, err := syntax.Parse(src, syntax.Core())
		if err != nil {
			t.Fatal(err)
		}
		return r.RunPart(ctx, f)
	}
	if err := part(context.Background(), `kept=yes`); err != nil {
		t.Fatal(err)
	}
	stopped, cancel := context.WithCancel(context.Background())
	cancel()
	if err := part(stopped, `echo never`); !errors.Is(err, context.Canceled) {
		t.Fatalf("the canceled chunk returned %v, want context.Canceled", err)
	}
	if r.Exited() {
		t.Error("the shell reports itself exited; a canceled chunk is not a canceled session")
	}
	if err := part(context.Background(), `printf "[%s]" "$kept"`); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "[yes]" {
		t.Errorf("out = %q, want [yes] — the chunk after the canceled one, with what the chunk before it set", got)
	}
}

// A canceled chunk does not run its EXIT trap.
//
// The trap is more of the script, and a caller that asked the run to stop has
// asked for no more of the script. Its own test because the trap fires on
// Run's error path, which is shared with the other thing that error can be —
// a construct this shell has not got — and there the trap belongs.
//
// The trap has to be *installed* before the cancellation, which is what makes
// this a deadline rather than an already-canceled context: a run canceled
// before its first command never reaches the `trap` command, so there is no
// trap to fire and the case would pass however the guard were written. That
// is measured — a first attempt at this test survived the mutant that runs
// the trap anyway.
func TestACanceledRunDoesNotRunItsExitTrap(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	out, _, err := runCancelable(t, ctx, `trap 'echo trapped' EXIT; echo before; while :; do :; done`, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if !strings.Contains(out, "before") {
		t.Fatalf("out = %q, want the trap to have been installed and the run to have started", out)
	}
	if strings.Contains(out, "trapped") {
		t.Errorf("out = %q, want the EXIT trap not to have run", out)
	}
	// The control: the same trap on a run nobody canceled does fire, so what
	// the assertion above is reading is the cancellation and not a trap that
	// never worked.
	out, _, err = runCancelable(t, context.Background(), `trap 'echo trapped' EXIT; echo done`, nil)
	if err != nil || !strings.Contains(out, "trapped") {
		t.Errorf("uncanceled: out = %q err = %v, want the trap to have run", out, err)
	}
}

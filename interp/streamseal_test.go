// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// bareWriter is a caller's io.Writer with no synchronization anywhere in it,
// and it is read without any either.
//
// That is the assertion rather than an oversight, and it is the difference
// from callersWriter in childlock_test.go. That one guards its own buffer, so
// what it catches is two of the shell's writers arriving at once; this one
// guards nothing, so what it catches is the shell leaving a goroutine able to
// reach the caller's writer *after the run has returned* — the missing
// happens-before edge rather than an overlap. It is the shape an embedder
// actually hands a Runner, and the shape CI reported the race on: a
// strings.Builder written under interp's lock and read by the test with no
// share of it.
type bareWriter struct{ b strings.Builder }

func (w *bareWriter) Write(p []byte) (int, error) { return w.b.Write(p) }

func (w *bareWriter) String() string { return w.b.String() }

// A body the shell walked away from cannot write to the caller's stream once
// the run has returned.
//
// #3969. A `<( … )` body runs on a goroutine of its own and at the top level
// nothing joins it — endHeldProcSubs joins r.bodies only when r.ownsBodies,
// which collectBodies sets around a *command substitution*, and heldProcSubs
// is the `exec 3> >(cat)` shape. So whatever such a body wrote to the shell's
// standard error reached the embedder's io.Writer with no edge to Run
// returning, which in a library is a data race and in a shell binary is a
// wash because both ends are fd 2.
//
// Joining it is not the answer, and that is measured rather than argued.
// Timing the shell's own exit — not a `$( … )` around it, which waits for the
// pipe the abandoned body still holds and reads 1.0s for every column
// whatever the shell did — `echo <(sleep 1; echo late >&2) >/dev/null` exits
// in 0.01s or less in bash 5.3, bash 3.2, ksh93 and zsh 5.9.2, with `late`
// landing in the redirected stderr about a second afterwards in all four. A
// join would hang the exit on `<(sleep 60)`; no shell in the panel does that.
//
// What the panel has instead is a *descriptor* that outlives the shell. The
// stand-in is the seal in streamseal.go: at the end of the run the shell
// takes the stream lock and marks it, which orders everything already written
// before the caller's next read and stops anything later from reaching the
// caller's writer at all.
//
// The body here is held at the gate by GuardConcurrent, which runs the work a
// spawn put on a goroutine and is the one hook that can hold it there without
// a timer. A `sleep` would be a window rather than a question — the same
// objection endHeldProcSubs records against probing its own shape with one —
// and this way the late write happens strictly after Run has returned, every
// run, on every machine.
func TestAnAbandonedBodyCannotWriteToTheCallersStreamAfterTheRunReturns(t *testing.T) {
	f, err := syntax.Parse(`: <(echo late >&2)`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}

	var once sync.Once
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})

	w := &bareWriter{}
	sem := testSemantics()
	dg := PosixDiagnostics()
	r := newTestRunner(t, &Runner{
		Stdout: w, Stderr: w, Semantics: &sem, Diagnostics: &dg, Env: testPATH(),
		GuardConcurrent: func(work func(), errs io.Writer) {
			once.Do(func() { close(started) })
			<-release
			work()
			close(finished)
		},
	})
	// Released whatever happens below, so a failing assertion leaves no
	// goroutine parked on a channel nobody is going to close.
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})

	deadline(t, "the run", func() {
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Error(err)
		}
	})
	if t.Failed() {
		return
	}

	deadline(t, "the body after the run", func() {
		<-started
		// The read an embedder makes, and the one the race detector
		// reported on: no lock, no channel, nothing but Run having
		// returned.
		before := w.String()
		close(release)
		<-finished
		if after := w.String(); after != before {
			t.Errorf("the stream gained %q after Run returned; a body the shell walked away from is still writing to the caller's writer",
				strings.TrimPrefix(after, before))
		}
	})
}

// And the seal does not eat what the shell wrote in time.
//
// The other half of #3969, in two shapes.
//
// The first is what CI actually reported: the body finishes *before* the run
// does — the `while` loop reads its pipe to end-of-file, which only comes
// when the body has returned — so its diagnostic belongs in what the caller
// reads back, and the seal must be what orders it rather than what drops it.
// It is read from the bare writer above with nothing synchronizing it,
// because that is the question. Under `-race` this is the case that reported
// `strings.(*Builder).String()` against `strings.(*Builder).Write()` under
// interp.(*lockedWriter).Write on run 35555666048.
//
// **It is the weaker of the two and it is worth saying which way.** Take the
// seal out entirely and this still passes on a laptop: the shell read that
// body's pipe to end-of-file, and the descriptor machinery underneath that
// read carries enough of an edge for the detector on this machine. It is the
// literal regression shape and it is the case CI's Linux leg tripped over,
// so it is kept as the shape; what it cannot be is the proof. The proof that
// nothing reaches the caller after the run is the test above, which fails on
// the content with no detector at all.
//
// The second shape is the seal coming *off* again, and that one discriminates
// on its own. A Runner is not always used once — RunPart is a chunk at a time
// and a caller may reach an end and carry on — so a seal that stayed on would
// leave the rest of the session writing into nothing. Nothing else in the
// tree would notice: every other test builds a Runner, runs it once and reads
// the buffer, which is precisely the shape a permanent seal is invisible to.
func TestTheSealDoesNotEatWhatTheShellWroteInTime(t *testing.T) {
	t.Run("a body that finished before the run did", func(t *testing.T) {
		f, err := syntax.Parse("while read -r l; do :; done < <(v=$(echo hi; for))\n", syntax.Core())
		if err != nil {
			t.Fatal(err)
		}
		w := &bareWriter{}
		sem := testSemantics()
		dg := PosixDiagnostics()
		r := newTestRunner(t, &Runner{
			Stdout: w, Stderr: w, Semantics: &sem, Diagnostics: &dg, Env: testPATH(),
		})
		deadline(t, "the run", func() {
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Error(err)
			}
		})
		if t.Failed() {
			return
		}
		// Unsynchronized, on purpose. See above.
		if got := w.String(); got == "" {
			t.Error("the caller's stream is empty: the body's diagnostic was written on its goroutine and the run has to hand it over")
		}
	})

	t.Run("a runner driven on past an end", func(t *testing.T) {
		// Both runs name a substitution, because the seal is over what a
		// body writes and nothing else: a second run whose output came from
		// the shell's own goroutine would pass whether the seal came off or
		// not. The loop is what makes the body's write land before the run
		// returns — it reads the pipe to end-of-file, which is the body
		// having finished.
		const src = "while read -r l; do :; done < <(echo %s >&2)\n"
		first, err := syntax.Parse(fmt.Sprintf(src, "once"), syntax.Core())
		if err != nil {
			t.Fatal(err)
		}
		f, err := syntax.Parse(fmt.Sprintf(src, "again"), syntax.Core())
		if err != nil {
			t.Fatal(err)
		}
		w := &bareWriter{}
		sem := testSemantics()
		dg := PosixDiagnostics()
		r := newTestRunner(t, &Runner{
			Stdout: w, Stderr: w, Semantics: &sem, Diagnostics: &dg, Env: testPATH(),
		})
		deadline(t, "two runs", func() {
			// The first one wraps the streams — a substitution is what puts
			// a guard over them — and then ends, which is what arms the
			// seal. Without it there would be no sealable writer at all.
			if _, err := r.Run(context.Background(), first); err != nil {
				t.Error(err)
				return
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Error(err)
			}
		})
		if t.Failed() {
			return
		}
		if got := w.String(); !strings.Contains(got, "again") {
			t.Errorf("the second run wrote %q, want it to contain %q: a shell asked for more work has its caller's streams back", got, "again")
		}
	})
}

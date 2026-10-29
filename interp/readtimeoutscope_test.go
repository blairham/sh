// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// What a `read -t` deadline is a deadline *for* (#644).
//
// One dialect bounds the wait for the stream to become **readable** and
// nothing after it: once a byte has arrived the line is read to its end
// however long that takes, and the answer is 0. The other two bound the whole
// read. The two agree for every timeout a normal script writes and disagree
// about *when* it expires, which is why the difference was recorded as a
// divergence rather than noticed as a bug.
//
// **These tests do not race a duration.** A test whose only evidence is that a
// deadline expired before some sleep finished passes on a fast machine and
// fails on a loaded one, and — worse — it passes for the wrong reason when the
// behavior is broken and the machine is slow.
//
// **The stream is a pipe, and the first byte is in it before the read
// begins.** A descriptor can be asked whether a byte is there without reading
// it, so the shell finds that byte however late it looks. These tests used to
// hand the shell an io.Reader instead, which can only be read on a goroutine,
// and a 1 ms deadline then raced that goroutine's first turn: on a loaded
// machine the read came back empty about one time in a hundred, the byte
// having been taken off the stream and thrown away (#7021).
//
// The rest of the line is released at a point that can only make each answer
// more certain. Where the deadline bounds the whole read, not until the run is
// over — no machine is slow enough to bring it inside the deadline, and a
// shell that ignored the deadline gets it after ten seconds and answers wrong
// rather than hanging. Where it bounds only readability, fifty milliseconds
// later, and the answer is the same whenever it comes.

// pipeHolding is a pipe whose read end is the shell's input, holding first,
// with rest written once release fires.
func pipeHolding(t *testing.T, first, rest string, release <-chan time.Time) *os.File {
	t.Helper()
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pw.WriteString(first); err != nil {
		t.Fatal(err)
	}
	finished := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-release:
			_, _ = pw.WriteString(rest)
		case <-finished:
		}
	}()
	t.Cleanup(func() {
		close(finished)
		wg.Wait()
		_ = pw.Close()
		_ = pr.Close()
	})
	return pr
}

// notDuringTheRun is a release no correct run is still going for. It exists so
// that a shell which kept reading past a whole-read deadline answers wrong
// instead of hanging the test binary.
func notDuringTheRun() <-chan time.Time { return time.After(10 * time.Second) }

func readTimeoutRun(t *testing.T, bounds Answer, release <-chan time.Time) (string, int) {
	t.Helper()
	stdin := pipeHolding(t, "a", "bc\n", release)
	return run(t, `v=old; read -t 0.001 -r v; echo "st=$? [$v]"`, func(r *Runner) {
		sem := CoreSemantics()
		sem.ReadOptions = "rt:"
		sem.ReadTimeoutKeepsWhatArrived = Yes
		sem.ReadTimeoutBoundsReadability = bounds
		r.Semantics = &sem
		r.Stdin = stdin
	})
}

// **The readability reading finishes the line.** The deadline was a
// millisecond and the rest of the line arrived fifty milliseconds later; the
// read still answers `abc` at 0, because the deadline stopped applying once
// the stream had something on it.
func TestATimeoutThatBoundsOnlyTheWaitForReadability(t *testing.T) {
	out, st := readTimeoutRun(t, Yes, time.After(50*time.Millisecond))
	if out != "st=0 [abc]\n" || st != 0 {
		t.Errorf("got %q status %d, want %q", out, st, "st=0 [abc]\n")
	}
}

// **The whole-read reading times out on the same stream**, which is what
// makes the test above evidence: both answers are reachable from one stream,
// so the first cannot be passing because nothing happened.
func TestATimeoutThatBoundsTheWholeRead(t *testing.T) {
	out, st := readTimeoutRun(t, No, notDuringTheRun())
	if out != "st=1 [a]\n" || st != 0 {
		t.Errorf("got %q status %d, want %q", out, st, "st=1 [a]\n")
	}
}

// A shell with no answer refuses rather than guessing, because the two
// answers give a script different text and a different status.
func TestATimeoutWithNoAnswerIsRefused(t *testing.T) {
	out, _ := readTimeoutRun(t, Unspecified, notDuringTheRun())
	if want := "bounding the wait for the first byte"; !strings.Contains(out, want) {
		t.Errorf("got %q, want the axis named", out)
	}
}

// **A byte already in the stream is taken, however short the deadline** —
// #7021's own case with the margin taken away. A deadline bounds a wait, and a
// byte that is already there makes nobody wait: bash 5.3.20 takes it at every
// deadline down to 100 µs, ten runs of ten, and only below its own overhead
// does the look become a race. A nanosecond is past before this shell can
// look, so the answer below is reachable only by looking anyway.
func TestAByteAlreadyThereIsTakenHoweverShortTheDeadline(t *testing.T) {
	for _, bounds := range []Answer{No, Yes} {
		// The readability reading reads on to the end of the line once it
		// has the first byte, so for it the line must have an end.
		held := map[Answer]string{No: "a", Yes: "abc\n"}[bounds]
		stdin := pipeHolding(t, held, "", notDuringTheRun())
		out, st := run(t, `v=old; read -t 0.000000001 -r v; echo "st=$? [$v]"`, func(r *Runner) {
			sem := CoreSemantics()
			sem.ReadOptions = "rt:"
			sem.ReadTimeoutKeepsWhatArrived = Yes
			sem.ReadTimeoutBoundsReadability = bounds
			r.Semantics = &sem
			r.Stdin = stdin
		})
		want := map[Answer]string{No: "st=1 [a]\n", Yes: "st=0 [abc]\n"}[bounds]
		if out != want || st != 0 {
			t.Errorf("bounds readability %v: got %q status %d, want %q", bounds, out, st, want)
		}
	}
}

// signalingWriter collects what a run writes and closes line once the first
// line of it is complete.
type signalingWriter struct {
	mu   sync.Mutex
	b    strings.Builder
	once sync.Once
	line chan struct{}
}

func (w *signalingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.b.Write(p)
	if strings.Contains(w.b.String(), "\n") {
		w.once.Do(func() { close(w.line) })
	}
	return n, err
}

func (w *signalingWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

// **What arrives after the deadline is the next reader's.** Nothing is in the
// pipe while the timed read waits; `xy` is written only once the shell has
// said the read is over, so the second read is the only thing that can take
// it. A read left parked on the pipe by the first one took the `x` and threw
// it away, and the second read got `y`. Measured 2026-10-29, every shell in
// the panel with the letter hands `xy` on; the per-dialect rows are in
// tests/cross.
func TestABytePastTheDeadlineIsLeftForTheNextRead(t *testing.T) {
	for _, keeps := range []Answer{Yes, No} {
		for _, bounds := range []Answer{Yes, No} {
			out := &signalingWriter{line: make(chan struct{})}
			release := make(chan time.Time)
			stdin := pipeHolding(t, "", "xy\n", release)
			go func() {
				select {
				case <-out.line:
				case <-time.After(10 * time.Second):
				}
				close(release)
			}()
			d := syntax.Core()
			f, err := syntax.Parse(`v=old; read -t 0.01 -r v; echo "st=$? [$v]"; read -r w; echo "next [$w]"`, d)
			if err != nil {
				t.Fatal(err)
			}
			sem := CoreSemantics()
			sem.ReadOptions = "rt:"
			sem.ReadTimeoutKeepsWhatArrived = keeps
			sem.ReadTimeoutBoundsReadability = bounds
			r := newTestRunner(t, &Runner{Stdout: out, Stderr: out, Semantics: &sem, Dialect: &d, Stdin: stdin})
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			first := map[Answer]string{Yes: "st=1 []\n", No: "st=1 [old]\n"}[keeps]
			if want := first + "next [xy]\n"; out.String() != want {
				t.Errorf("keeps %v, bounds readability %v: got %q, want %q", keeps, bounds, out.String(), want)
			}
		}
	}
}

// **A timed read stops when its context does.** The reader goroutine a
// descriptor no longer uses gave up the moment its context ended, and an
// embedder cancels a running line to kill it — termhost's Kill does exactly
// that — so a read waiting on a pipe must not run out a thirty-second deadline
// first.
func TestATimedReadOnAPipeStopsWhenItsContextDoes(t *testing.T) {
	out := &signalingWriter{line: make(chan struct{})}
	stdin := pipeHolding(t, "", "", notDuringTheRun())
	d := syntax.Core()
	f, err := syntax.Parse(`echo started; read -t 30 -r v; echo "st=$?"`, d)
	if err != nil {
		t.Fatal(err)
	}
	sem := CoreSemantics()
	sem.ReadOptions = "rt:"
	sem.ReadTimeoutKeepsWhatArrived = No
	sem.ReadTimeoutBoundsReadability = No
	r := newTestRunner(t, &Runner{Stdout: out, Stderr: out, Semantics: &sem, Dialect: &d, Stdin: stdin})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	returned := make(chan struct{})
	go func() {
		defer close(returned)
		_, _ = r.Run(ctx, f)
	}()
	<-out.line
	// The read begins a statement after the line is written. Waiting a
	// little longer only makes it more certain the read is the thing being
	// canceled, never less.
	time.Sleep(100 * time.Millisecond)
	canceled := time.Now()
	cancel()
	select {
	case <-returned:
	case <-time.After(10 * time.Second):
		t.Fatalf("the run was still going ten seconds after its context ended (%q)", out.String())
	}
	if took := time.Since(canceled); took > 5*time.Second {
		t.Errorf("the run took %v to stop after its context ended", took)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

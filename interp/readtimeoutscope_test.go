// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"io"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
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
// The shape here is one-sided instead. The deadline is a *millisecond*, and
// the second byte is not released until the test has slept for fifty of them.
// A slower machine only lengthens that wait, so the deadline is more surely
// past, never less — and the assertion is that the read succeeded anyway. It
// can be satisfied only by a deadline that stopped applying, which is the
// mechanism under test rather than a race against it.

// dripReader delivers one byte, then blocks until the test lets it go, then
// delivers the rest.
//
// A reader rather than a pipe with sleeps in it, so that the *release* is the
// test's own act and not a duration anybody has to guess at.
type dripReader struct {
	first   []byte
	rest    []byte
	release <-chan struct{}
	gave    bool
}

func (d *dripReader) Read(p []byte) (int, error) {
	if !d.gave {
		d.gave = true
		n := copy(p, d.first)
		return n, nil
	}
	if d.release != nil {
		<-d.release
		d.release = nil
	}
	if len(d.rest) == 0 {
		return 0, io.EOF
	}
	n := copy(p, d.rest)
	d.rest = d.rest[n:]
	return n, nil
}

func readTimeoutRun(t *testing.T, bounds Answer) (string, int) {
	t.Helper()
	release := make(chan struct{})
	go func() {
		// Fifty times the deadline, and the direction is what matters: a
		// machine that takes longer makes the deadline more certainly past.
		time.Sleep(50 * time.Millisecond)
		close(release)
	}()
	return run(t, `v=old; read -t 0.001 -r v; echo "st=$? [$v]"`, func(r *Runner) {
		sem := CoreSemantics()
		sem.ReadOptions = "rt:"
		sem.ReadTimeoutKeepsWhatArrived = Yes
		sem.ReadTimeoutBoundsReadability = bounds
		r.Semantics = &sem
		r.Stdin = &dripReader{first: []byte("a"), rest: []byte("bc\n"), release: release}
	})
}

// **The readability reading finishes the line.** The deadline was a
// millisecond and the rest of the line arrived fifty milliseconds later; the
// read still answers `abc` at 0, because the deadline stopped applying once
// the stream had something on it.
func TestATimeoutThatBoundsOnlyTheWaitForReadability(t *testing.T) {
	out, st := readTimeoutRun(t, Yes)
	if out != "st=0 [abc]\n" || st != 0 {
		t.Errorf("got %q status %d, want %q", out, st, "st=0 [abc]\n")
	}
}

// **The whole-read reading times out on the same stream**, which is what
// makes the test above evidence: both answers are reachable from one
// arrangement, so the first cannot be passing because nothing happened.
func TestATimeoutThatBoundsTheWholeRead(t *testing.T) {
	out, st := readTimeoutRun(t, No)
	if out != "st=1 [a]\n" || st != 0 {
		t.Errorf("got %q status %d, want %q", out, st, "st=1 [a]\n")
	}
}

// A shell with no answer refuses rather than guessing, because the two
// answers give a script different text and a different status.
func TestATimeoutWithNoAnswerIsRefused(t *testing.T) {
	out, _ := readTimeoutRun(t, Unspecified)
	if want := "bounding the wait for the first byte"; !contains(out, want) {
		t.Errorf("got %q, want the axis named", out)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

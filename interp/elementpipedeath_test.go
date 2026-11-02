// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// The kernel's copies of a SIGPIPE a builtin's own writes raised are not the
// outer shell's to handle (#7288).
//
// The outer shell has a handler, so the process is subscribed and the runtime
// forwards every copy. An element does not inherit that handler, so it dies of
// the default where it stands. No panel shell runs the outer handler there: a
// real element is a process of its own, and its signal never reaches the shell
// that started it.
//
// The stream is raisingBrokenPipe from brokenpartway_test.go, which puts the
// kernel's copies on the channel from inside each write, before the shell can
// record anything: one copy for a write that found the reader gone, two for a
// write the reader left part-way through. The outer handler prints, so a copy
// that got past the credits is seen rather than run as `:`.

// outerHandlerRuns runs src under an outer handler that prints, with standard
// output a stream that raises copies on every write and fails it after wrote
// bytes. It reports how often the handler ran, and standard error whole.
func outerHandlerRuns(t *testing.T, src string, copies, wrote int) (int, string) {
	t.Helper()
	sem := PosixSemantics()
	// Every arrival runs its handler, so a stray copy is counted rather than
	// folded into a turn some other arrival already has. Left unanswered, two
	// arrivals in one batch would run nothing at all and hide the second.
	sem.WaitingSignalArrivingAgainRunsOnce = No
	var errOut bytes.Buffer
	r := newTestRunner(t, &Runner{Stderr: &errOut, Semantics: &sem})
	r.Stdout = &raisingBrokenPipe{r: r, copies: copies, wrote: wrote}
	f, err := syntax.Parse(`trap 'echo outer >&2' PIPE; `+src+`; echo after >&2`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	out := errOut.String()
	if !strings.Contains(out, "after") {
		t.Fatalf("%q: stderr = %q, want the shell to carry on", src, out)
	}
	return strings.Count(out, "outer"), out
}

// elementDeathOuterRuns is outerHandlerRuns over an element that has to die at
// its write, since the death is the path under test.
func elementDeathOuterRuns(t *testing.T, src string, copies, wrote int) (int, string) {
	t.Helper()
	n, out := outerHandlerRuns(t, src, copies, wrote)
	if strings.Contains(out, "reached") {
		t.Fatalf("%q: stderr = %q, want the element dead at its write", src, out)
	}
	return n, out
}

// The two places an element is a copy of the shell with the handler put back
// to its default, a subshell and a pipeline element, each with one write and
// with several: `printf` writes once per pass over its format. The last element
// of a pipeline writes to the shell's own standard output, the raising stream.
var elementsThatDieOfTheDefault = []struct {
	src    string
	writes int
}{
	{`( echo hi; echo reached >&2 )`, 1},
	{`: | { echo hi; echo reached >&2; }`, 1},
	{`( printf '%s\n' a b c; echo reached >&2 )`, 3},
	{`: | { printf '%s\n' a b c; echo reached >&2; }`, 3},
}

func TestAnElementsDeathByThePipeRunsNoOuterHandler(t *testing.T) {
	for _, e := range elementsThatDieOfTheDefault {
		for _, c := range []struct {
			name          string
			copies, wrote int
		}{
			{"the reader gone before the write", 1, 0},
			{"the reader gone part-way through", 2, 2},
		} {
			if n, errOut := elementDeathOuterRuns(t, e.src, c.copies, c.wrote); n != 0 {
				t.Errorf("%q, %s: the outer handler ran %d times, want never (stderr %q)", e.src, c.name, n, errOut)
			}
		}
	}
}

// The control: a copy beyond what each write raised is a signal from outside,
// and the outer handler runs once for each of them. Without this the row above
// could pass because no copy ever reached the shell at all, and with several
// writes it says the credit is counted per write and not merely large.
func TestACopyBeyondTheElementsDeathStillRunsTheOuterHandler(t *testing.T) {
	for _, e := range elementsThatDieOfTheDefault {
		for _, c := range []struct {
			name          string
			copies, wrote int
		}{
			{"the reader gone before the write", 2, 0},
			{"the reader gone part-way through", 3, 2},
		} {
			if n, errOut := elementDeathOuterRuns(t, e.src, c.copies, c.wrote); n != e.writes {
				t.Errorf("%q, %s: the outer handler ran %d times, want %d, once per copy from outside (stderr %q)",
					e.src, c.name, n, e.writes, errOut)
			}
		}
	}
}

// The same count on the road where the shell handles the signal itself: one
// `printf` failing three writes is one arrival to the script, as one failed
// `echo` is, however many copies the three raised. Measured 2026-11-01 for an
// element's own handler, which bash 5.3.20, bash 3.2, dash and ksh93 run once
// for a printf whose four passes each failed.
func TestAPrintfFailingEveryPassRunsTheHandlerOnce(t *testing.T) {
	for _, c := range []struct {
		name          string
		copies, wrote int
	}{
		{"the reader gone before the write", 1, 0},
		{"the reader gone part-way through", 2, 2},
	} {
		if n, errOut := outerHandlerRuns(t, `printf '%s\n' a b c`, c.copies, c.wrote); n != 1 {
			t.Errorf("%s: the handler ran %d times, want once (stderr %q)", c.name, n, errOut)
		}
	}
}

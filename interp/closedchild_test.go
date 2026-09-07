// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A closed descriptor and an empty one are different things, and the child is
// where they were the same thing (#1260).
//
// os/exec builds a pipe for a stream that is not a file and copies through it,
// so handing a child the shell's marker for "closed" gave it a pipe that shut
// on the first failed copy — end-of-file, at status 0, in silence. Every shell
// in the panel fails the read with EBADF and says so, so the status alone is
// not what these assert: a believable success is the whole of the bug, and a
// test that only checked the output would have passed before the fix.
func TestAClosedStdinIsClosedForAnExternalCommand(t *testing.T) {
	out, errOut, _ := runSplit(t, `exec 0<&-; /bin/cat; echo "st=$?"`)
	// The exact status, not a prefix of one. A shell that hands the child a
	// pipe instead gets end-of-file *and* a failed copy on its own side, and
	// reports that as 126 — which is a different failure with a different
	// cause, and `st=126` contains `st=1`.
	if out != "st=1\n" {
		t.Errorf("stdout = %q, want exactly st=1; stderr %q", out, errOut)
	}
	if errOut == "" {
		t.Error("the command said nothing; a closed descriptor is diagnosed, an empty one is not")
	}
}

// The writing half, which is its own measurement rather than a consequence of
// the reading one: a write into a pipe nobody reads succeeds.
func TestAClosedStdoutIsClosedForAnExternalCommand(t *testing.T) {
	out, errOut, _ := runSplit(t, `( exec 1>&-; /bin/echo hi ); echo "st=$?"`)
	if out != "st=1\n" {
		t.Errorf("stdout = %q, want exactly st=1; stderr %q", out, errOut)
	}
	if errOut == "" {
		t.Error("the command said nothing about a write it could not make")
	}
	if strings.Contains(out, "hi") {
		t.Errorf("stdout = %q, the closed stream still carried the command's text", out)
	}
}

// The control that makes the two above discriminating. A descriptor with
// nothing in it reads end-of-file and the command succeeds in silence, which
// is exactly the answer the closed forms used to give.
func TestAnEmptyStdinIsNotAClosedOne(t *testing.T) {
	out, errOut, _ := runSplit(t, `exec 0</dev/null; /bin/cat; echo "st=$?"`)
	if out != "st=0\n" {
		t.Errorf("stdout = %q, want exactly st=0; stderr %q", out, errOut)
	}
	if errOut != "" {
		t.Errorf("stderr = %q, want nothing said about a descriptor that is merely empty", errOut)
	}
}

// Without `exec`, so the rule is about what a command is handed rather than
// about what the shell parks across commands.
func TestAPerCommandCloseReachesAnExternalCommand(t *testing.T) {
	out, errOut, _ := runSplit(t, `/bin/cat <&-; echo "st=$?"`)
	if out != "st=1\n" {
		t.Errorf("stdout = %q, want exactly st=1; stderr %q", out, errOut)
	}
	if errOut == "" {
		t.Error("the command said nothing about a descriptor it could not read")
	}
}

// The marker has to survive the guard a background job puts over the shell's
// input. That wraps anything that is not a real file, so that two children
// copying from one caller's stream do not race — and a wrapped closedFd is an
// ordinary stream again, which puts the pipe back.
//
// Asked under the two answers that hand the job the descriptor at all, because
// a `&` job's standard input is itself an axis (#1287): dash and bash replace
// it with an empty stream whatever it was, so there is no marker left to
// survive anything, and ksh93 and zsh reach the job with the closed descriptor
// in hand. The status is asserted exactly for the reason the rows above assert
// it: a job whose child was handed a pipe still fails, at 126 and for the
// shell's own reason, which is not the failure being pinned.
func TestAClosedStreamStaysClosedThroughABackgroundJobsInputGuard(t *testing.T) {
	for _, answer := range []BackgroundJobInputPolicy{
		BackgroundJobInputEmptyUnlessClosed,
		BackgroundJobInputIsTheShells,
	} {
		out, errOut, _ := runSplitWithBackgroundInput(t,
			`exec 0<&-; /bin/cat & wait "$!"; echo "st=$?"`, answer)
		if out != "st=1\n" {
			t.Errorf("%v: stdout = %q, want exactly st=1; stderr %q", answer, out, errOut)
		}
		if errOut == "" {
			t.Errorf("%v: the background command said nothing about a descriptor it could not read", answer)
		}
	}
}

// And the control that makes the row above a measurement of the guard rather
// than of the axis: the answer that substitutes an empty stream reads
// end-of-file and says nothing, which is what dash and bash do — measured
// 2026-09-07, `exec 0<&-; /bin/cat & wait` is silent at 0 there and
// `cat: stdin: Bad file descriptor` in ksh93u+ and zsh 5.9.2.
func TestABackgroundJobsEmptyInputReplacesEvenAClosedDescriptor(t *testing.T) {
	out, errOut, _ := runSplitWithBackgroundInput(t,
		`exec 0<&-; /bin/cat & wait "$!"; echo "st=$?"`, BackgroundJobInputEmpty)
	if out != "st=0\n" {
		t.Errorf("stdout = %q, want exactly st=0; stderr %q", out, errOut)
	}
	if errOut != "" {
		t.Errorf("stderr = %q, want nothing said: the job was handed an empty stream, not a closed one", errOut)
	}
}

func TestAClosedStreamStaysClosedThroughABackgroundJobsOutputGuard(t *testing.T) {
	out, errOut, _ := runSplit(t, `( exec 1>&-; /bin/echo hi & wait "$!" ); echo "st=$?"`)
	if out != "st=1\n" {
		t.Errorf("stdout = %q, want exactly st=1; stderr %q", out, errOut)
	}
	if errOut == "" {
		t.Error("the background command said nothing about a write it could not make")
	}
}

// A closed standard error is the third stream and needs asking about
// separately: nothing else in this file writes to one.
//
// It is asked a different way, because a *write* cannot tell the two answers
// apart here. A child given a pipe for its standard error fails that write
// too — the shell's own copy out of the pipe fails and shuts it, so the child
// meets a dead pipe rather than a closed descriptor — and the status it dies
// with then belongs to whatever `/bin/sh` is on the machine. A first draft
// pinned that number and passed on one platform and failed on the other.
//
// So the child is asked whether the descriptor is *open*, which is the thing
// actually in question: duplicating it fails immediately with EBADF when it
// is closed and succeeds when it is a pipe, whatever is buffered and whoever
// wins the race. Unanimous across the panel, and the subshell keeps the
// failure from ending the child before it can report.
const stderrIsOpenToTheChild = `/bin/sh -c "( exec 3>&2 ) && echo dup=ok || echo dup=failed"`

func TestAClosedStderrIsClosedForAnExternalCommand(t *testing.T) {
	out, _, _ := runSplit(t, `exec 2>&-; `+stderrIsOpenToTheChild)
	if out != "dup=failed\n" {
		t.Errorf("stdout = %q, want dup=failed: the child could still duplicate a descriptor the script closed", out)
	}
}

// The control that makes the two rows above discriminating, and the reason
// the probe is not simply always-false: an ordinary standard error is open to
// the child and duplicates fine.
func TestAnOpenStderrIsStillOpenToTheChild(t *testing.T) {
	out, _, _ := runSplit(t, stderrIsOpenToTheChild)
	if out != "dup=ok\n" {
		t.Errorf("stdout = %q, want dup=ok: a stream nobody closed must reach the child open", out)
	}
}

// `exec cmd` in a Runner with no replacement hook — an embedded shell, and
// every subshell — stands in for a replacement with an ordinary child, so it
// has to hand over the same three streams. It is a second call site of the
// same rule, which is exactly where a fix gets forgotten.
func TestAClosedStdinIsClosedForAReplacementStandIn(t *testing.T) {
	out, errOut, st := runSplit(t, `exec 0<&-; exec /bin/cat`)
	if st != 1 {
		t.Errorf("status = %d, want the read to fail at 1; stdout %q, stderr %q", st, out, errOut)
	}
	if errOut == "" {
		t.Error("the stand-in said nothing about a descriptor it could not read")
	}
}

func TestAClosedStdoutIsClosedForAReplacementStandIn(t *testing.T) {
	out, errOut, st := runSplit(t, `exec 1>&-; exec /bin/echo hi`)
	if st != 1 {
		t.Errorf("status = %d, want the write to fail at 1; stdout %q, stderr %q", st, out, errOut)
	}
	if errOut == "" {
		t.Error("the stand-in said nothing about a write it could not make")
	}
	if strings.Contains(out, "hi") {
		t.Errorf("stdout = %q, the closed stream still carried the command's text", out)
	}
}

func TestAClosedStderrIsClosedForAReplacementStandIn(t *testing.T) {
	out, _, _ := runSplit(t, `exec 2>&-; exec `+stderrIsOpenToTheChild)
	if out != "dup=failed\n" {
		t.Errorf("stdout = %q, want dup=failed: the stand-in could still duplicate a descriptor the script closed", out)
	}
}

// runSplitWithBackgroundInput is runSplit with one axis answered: what a job
// started with `&` reads for standard input.
func runSplitWithBackgroundInput(t *testing.T, src string, answer BackgroundJobInputPolicy) (out, errOut string, status int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var o, e bytes.Buffer
	sem := testSemantics()
	sem.BackgroundJobInput = answer
	r := newTestRunner(t, &Runner{Stdout: &o, Stderr: &e, Semantics: &sem})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatal(rerr)
	}
	return o.String(), e.String(), st
}

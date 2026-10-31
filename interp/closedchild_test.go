// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/blairham/sh/internal/testenv"
	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A closed descriptor and an empty one are different things, and the child is
// where they were the same thing (#1260).
//
// os/exec builds a pipe for a stream that is not a file and copies through it,
// so handing a child the shell's marker for "closed" gave it a pipe that shut
// on the first failed copy — end-of-file, at status 0, in silence. Every shell
// in the panel hands the child a closed descriptor instead.
//
// The child each row hands a stream to asks whether that stream is open,
// rather than reading or writing it and being judged on what it says about
// the result. A program's complaint is the program's policy: these rows used
// /bin/cat and /bin/echo until Ubuntu 26.04 made both uutils coreutils, which
// answer a closed stream silently at 0, exactly as an empty one (#7199).
// testenv.DescriptorProbe has the measurements, including why the child is
// not this test binary.
//
// The probe exits 0 whichever answer it gives, so a row asserts the status
// beside the answer and wants 0: anything else is the shell's own. A shell
// that hands the child a pipe instead fails its own copy out of the closed
// stream, and reports that as 126 — a different failure with a different
// cause, and one these rows would otherwise read past.
var (
	stdinIsOpenToTheChild = testenv.DescriptorProbe(0, "")
	// Answered on standard error, since standard output is the question.
	stdoutIsOpenToTheChild = testenv.DescriptorProbe(1, ">&2")
	stderrIsOpenToTheChild = testenv.DescriptorProbe(2, "")
)

const (
	childFoundItOpen   = testenv.DescriptorOpen + "\n"
	childFoundItClosed = testenv.DescriptorClosed + "\n"
)

// The reading half.
func TestAClosedStdinIsClosedForAnExternalCommand(t *testing.T) {
	out, errOut, _ := runSplit(t, `exec 0<&-; `+stdinIsOpenToTheChild+`; echo "st=$?"`)
	if want := childFoundItClosed + "st=0\n"; out != want {
		t.Errorf("stdout = %q, want %q; stderr %q", out, want, errOut)
	}
}

// The writing half, which is its own measurement rather than a consequence of
// the reading one: a write into a pipe nobody reads succeeds.
func TestAClosedStdoutIsClosedForAnExternalCommand(t *testing.T) {
	out, errOut, _ := runSplit(t, `( exec 1>&-; `+stdoutIsOpenToTheChild+` ); echo "st=$?"`)
	if errOut != childFoundItClosed {
		t.Errorf("stderr = %q, want %q: the child could still duplicate a standard output the script closed",
			errOut, childFoundItClosed)
	}
	if out != "st=0\n" {
		t.Errorf("stdout = %q, want exactly st=0", out)
	}
}

// The control that makes the rows above discriminating: a descriptor with
// nothing in it is open, and the child says so. Both of the empties a shell
// can mistake for a closed one — the null device, and the pipe with nothing
// behind it that #1260 handed over.
func TestAnEmptyStdinIsNotAClosedOne(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"null device", `exec 0</dev/null; ` + stdinIsOpenToTheChild + `; echo "st=$?"`},
		{"empty pipe", `: | ` + stdinIsOpenToTheChild + `; echo "st=$?"`},
	} {
		out, errOut, _ := runSplit(t, tc.src)
		if want := childFoundItOpen + "st=0\n"; out != want {
			t.Errorf("%s: stdout = %q, want %q; stderr %q", tc.name, out, want, errOut)
		}
	}
}

// Without `exec`, so the rule is about what a command is handed rather than
// about what the shell parks across commands.
func TestAPerCommandCloseReachesAnExternalCommand(t *testing.T) {
	out, errOut, _ := runSplit(t, stdinIsOpenToTheChild+` <&-; echo "st=$?"`)
	if want := childFoundItClosed + "st=0\n"; out != want {
		t.Errorf("stdout = %q, want %q; stderr %q", out, want, errOut)
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
// it: a job whose child was handed a pipe fails at 126 for the shell's own
// reason, which is not the answer being pinned.
func TestAClosedStreamStaysClosedThroughABackgroundJobsInputGuard(t *testing.T) {
	for _, answer := range []BackgroundJobInputPolicy{
		BackgroundJobInputEmptyUnlessClosed,
		BackgroundJobInputIsTheShells,
	} {
		out, errOut, _ := runSplitWithBackgroundInput(t,
			`exec 0<&-; `+stdinIsOpenToTheChild+` & wait "$!"; echo "st=$?"`, answer)
		if want := childFoundItClosed + "st=0\n"; out != want {
			t.Errorf("%v: stdout = %q, want %q; stderr %q", answer, out, want, errOut)
		}
	}
}

// And the control that makes the row above a measurement of the guard rather
// than of the axis: the answer that substitutes an empty stream hands the job
// an open one, which is what dash and bash do — measured 2026-09-07,
// `exec 0<&-; /bin/cat & wait` is silent at 0 there and
// `cat: stdin: Bad file descriptor` in ksh93u+ and zsh 5.9.2.
func TestABackgroundJobsEmptyInputReplacesEvenAClosedDescriptor(t *testing.T) {
	out, errOut, _ := runSplitWithBackgroundInput(t,
		`exec 0<&-; `+stdinIsOpenToTheChild+` & wait "$!"; echo "st=$?"`, BackgroundJobInputEmpty)
	if want := childFoundItOpen + "st=0\n"; out != want {
		t.Errorf("stdout = %q, want %q: the job was handed an empty stream, not a closed one; stderr %q",
			out, want, errOut)
	}
}

func TestAClosedStreamStaysClosedThroughABackgroundJobsOutputGuard(t *testing.T) {
	out, errOut, _ := runSplit(t, `( exec 1>&-; `+stdoutIsOpenToTheChild+` & wait "$!" ); echo "st=$?"`)
	if errOut != childFoundItClosed {
		t.Errorf("stderr = %q, want %q: the background command could still duplicate a closed standard output",
			errOut, childFoundItClosed)
	}
	if out != "st=0\n" {
		t.Errorf("stdout = %q, want exactly st=0", out)
	}
}

// The control for the writing rows: an ordinary standard output — a pipe here,
// since the test's buffer is not a file — is open to the child.
func TestAnOpenStdoutIsStillOpenToTheChild(t *testing.T) {
	out, errOut, _ := runSplit(t, stdoutIsOpenToTheChild+`; echo "st=$?"`)
	if errOut != childFoundItOpen {
		t.Errorf("stderr = %q, want %q: a stream nobody closed must reach the child open", errOut, childFoundItOpen)
	}
	if out != "st=0\n" {
		t.Errorf("stdout = %q, want exactly st=0", out)
	}
}

// A closed standard error is the third stream and needs asking about
// separately: nothing else in this file writes to one.
//
// It was the first row asked this way, because a *write* could not tell the
// two answers apart even with GNU's programs. A child given a pipe for its
// standard error fails that write too — the shell's own copy out of the pipe
// fails and shuts it, so the child meets a dead pipe rather than a closed
// descriptor — and the status it dies with then belongs to whatever `/bin/sh`
// is on the machine. A first draft pinned that number and passed on one
// platform and failed on the other.
func TestAClosedStderrIsClosedForAnExternalCommand(t *testing.T) {
	out, _, _ := runSplit(t, `exec 2>&-; `+stderrIsOpenToTheChild)
	if out != childFoundItClosed {
		t.Errorf("stdout = %q, want %q: the child could still duplicate a descriptor the script closed",
			out, childFoundItClosed)
	}
}

// The control that makes the stderr row discriminating, and the reason the
// probe is not simply always-false: an ordinary standard error is open to the
// child and duplicates fine.
func TestAnOpenStderrIsStillOpenToTheChild(t *testing.T) {
	out, _, _ := runSplit(t, stderrIsOpenToTheChild)
	if out != childFoundItOpen {
		t.Errorf("stdout = %q, want %q: a stream nobody closed must reach the child open", out, childFoundItOpen)
	}
}

// `exec cmd` in a Runner with no replacement hook — an embedded shell, and
// every subshell — stands in for a replacement with an ordinary child, so it
// has to hand over the same three streams. It is a second call site of the
// same rule, which is exactly where a fix gets forgotten.
//
// The status is the stand-in's, which is the probe's, which is 0 either way.
func TestAClosedStdinIsClosedForAReplacementStandIn(t *testing.T) {
	out, errOut, st := runSplit(t, `exec 0<&-; exec `+stdinIsOpenToTheChild)
	if out != childFoundItClosed || st != 0 {
		t.Errorf("stdout = %q, status %d, want %q at 0; stderr %q", out, st, childFoundItClosed, errOut)
	}
}

func TestAClosedStdoutIsClosedForAReplacementStandIn(t *testing.T) {
	out, errOut, st := runSplit(t, `exec 1>&-; exec `+stdoutIsOpenToTheChild)
	if errOut != childFoundItClosed || st != 0 {
		t.Errorf("stderr = %q, status %d, want %q at 0; stdout %q", errOut, st, childFoundItClosed, out)
	}
	if out != "" {
		t.Errorf("stdout = %q, the closed stream still carried something", out)
	}
}

func TestAClosedStderrIsClosedForAReplacementStandIn(t *testing.T) {
	out, _, _ := runSplit(t, `exec 2>&-; exec `+stderrIsOpenToTheChild)
	if out != childFoundItClosed {
		t.Errorf("stdout = %q, want %q: the stand-in could still duplicate a descriptor the script closed",
			out, childFoundItClosed)
	}
}

// runSplitWithBackgroundInput is runSplit with one axis answered: what a job
// started with `&` reads for standard input.
func runSplitWithBackgroundInput(
	t *testing.T,
	src string,
	answer BackgroundJobInputPolicy,
) (out, errOut string, status int) {
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

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A duplication reads its source before the command runs, so a source the
// script closed fails the *redirection* — and that is a different event from
// the write failure in closedwrite_test.go, however alike the two errnos look.
//
// The distinction is the whole of #1345. `closedFd` answers EBADF to a read
// and to a write, so a duplication used to copy it and succeed, and the
// command that followed failed on the write it happened to attempt. That is
// the right answer reached the wrong way, and it stops being the right answer
// the moment the command writes nothing at all: see the second test, which is
// the one that cannot pass by accident.
//
// Every test here names an axis or a stream and never a shell, so the file
// belongs in the core rather than under dialect/.

// closedDupRun runs a snippet with the write-failure axis answered, so that a
// stray *write* failure would be visible rather than swallowed — the point
// being that these cases must fail before any write is attempted.
func closedDupRun(t *testing.T, src string, diag Diagnostics) (out, errOut string, status int) {
	t.Helper()
	sem := PosixSemantics()
	sem.BuiltinWriteErrorFailsTheCommand = Yes
	diag.BuiltinWriteError = "%[1]s: write error: %[2]s"
	return runClosedWrite(t, src, sem, diag)
}

func TestADuplicationFromAClosedStreamFailsTheRedirection(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		// Each of the three named streams, as both a source and a target,
		// because the resolution that was wrong is shared by all of them and
		// a test on one alone would not say so.
		{"stderr closed, duplicated onto stdout", `exec 2>&-; echo z >&2; echo st=$?`},
		{"stdout closed, duplicated onto stderr", `exec 1>&-; echo z 2>&1; echo st=$? >&2`},
		{"stdin closed, duplicated onto another reader", `exec 0<&-; cat <&0; echo st=$?`},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errOut, _ := closedDupRun(t, c.src, Diagnostics{})
			if !strings.Contains(out+errOut, "st=1") {
				t.Errorf("out %q, err %q — want st=1: a closed source is not a descriptor", out, errOut)
			}
		})
	}
}

// The discriminating one. `true` writes nothing, so a shell that fails this
// can only be failing the duplication; a shell that models a closed stream as
// something writable answers 0 here while answering the test above correctly,
// which is exactly the state #1345 found.
func TestADuplicationFromAClosedStreamFailsWithNothingToWrite(t *testing.T) {
	out, errOut, _ := closedDupRun(t, `exec 2>&-; true >&2; echo st=$?`, Diagnostics{})
	if out != "st=1\n" {
		t.Errorf("stdout = %q, want st=1 — the command never ran, so it had no write to fail", out)
	}
	if errOut != "" {
		t.Errorf("stderr = %q, want nothing: a write error would name the builtin, and there was no write", errOut)
	}
}

// The control, and it is the reason the two above are about the close rather
// than about `>&2` at all: the duplication is read first, so the stream it
// copied outlives the close that follows it.
func TestADuplicationReadBeforeTheCloseSurvivesIt(t *testing.T) {
	out, errOut, st := closedDupRun(t, `echo z >&2 2>&-; echo st=$?`, Diagnostics{})
	if errOut != "z\n" {
		t.Errorf("stderr = %q, want the line to have reached the copy taken before the close", errOut)
	}
	if out != "st=0\n" || st != 0 {
		t.Errorf("stdout = %q, status %d — want a success", out, st)
	}
}

// The failure is reported the way a redirection failure is, and carries the
// dialect's number rather than a fixed 1. It said 1 whatever was answered,
// which made a duplication and an open disagree about the same event.
func TestADuplicationFailureCarriesTheRedirectFailureStatus(t *testing.T) {
	for _, c := range []struct {
		name string
		diag Diagnostics
		want string
	}{
		{"the substrate's own", Diagnostics{}, "st=1"},
		{"a dialect's own number", Diagnostics{RedirectFailureStatus: 2}, "st=2"},
	} {
		t.Run(c.name, func(t *testing.T) {
			// A number never opened, which is the same refusal by the other
			// route: both arrive at errBadFd, so grading one grades the pair.
			out, _, _ := closedDupRun(t, `echo z >&7; echo st=$?`, c.diag)
			if !strings.Contains(out, c.want) {
				t.Errorf("stdout = %q, want %s", out, c.want)
			}
		})
	}
}

// A closed source is named the way an unopened one is, so the two spellings of
// "not a descriptor" produce one sentence rather than two.
func TestAClosedDuplicationSourceIsNamedByItsNumber(t *testing.T) {
	diag := Diagnostics{}
	_, errOut, _ := closedDupRun(t, `exec 1>&-; exec 3>&1`, diag)
	if !strings.Contains(errOut, "1: Bad file descriptor") {
		t.Errorf("stderr = %q, want the closed source named by its number", errOut)
	}
	// And the descriptor the failed duplication would have filled is not
	// filled: it stays as absent as it was, rather than holding a copy of the
	// closed stream. Without this the close propagated one number at a time.
	_, errOut, _ = closedDupRun(t, `exec 1>&- 2>&-; exec 3>&1; echo x >&3`, Diagnostics{})
	if strings.Contains(errOut, "write error") {
		t.Errorf("stderr = %q, want a bad descriptor rather than a write into one", errOut)
	}
}

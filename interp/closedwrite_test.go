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

// runClosedWrite runs a snippet with the write-failure axis and wording under
// the test's control, keeping the two streams apart — every question here is
// about which stream carried what.
func runClosedWrite(t *testing.T, src string, sem Semantics, diag Diagnostics) (out, errOut string, status int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var o, e bytes.Buffer
	r := newTestRunner(t, &Runner{Stdout: &o, Stderr: &e, Semantics: &sem, Diagnostics: &diag})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatal(rerr)
	}
	return o.String(), e.String(), st
}

// failingWrites answers the axis yes and gives the wording a shape whose two
// verbs — the builtin and the reason — are both visible in the result.
func failingWrites() (Semantics, Diagnostics) {
	sem := PosixSemantics()
	sem.BuiltinWriteErrorFailsTheCommand = Yes
	return sem, Diagnostics{BuiltinWriteError: "%[1]s: write error: %[2]s"}
}

func TestFailedWriteFailsTheBuiltin(t *testing.T) {
	sem, diag := failingWrites()
	out, errOut, _ := runClosedWrite(t, `echo hi >&-; echo st=$?`, sem, diag)
	if out != "st=1\n" {
		t.Errorf("stdout = %q, want the failure's status and nothing else", out)
	}
	if want := "echo: write error: Bad file descriptor"; !strings.Contains(errOut, want) {
		t.Errorf("stderr = %q, want it to carry %q", errOut, want)
	}
}

func TestFailedWriteNamesTheBuiltinThatWrote(t *testing.T) {
	sem, diag := failingWrites()
	_, errOut, st := runClosedWrite(t, `printf x >&-`, sem, diag)
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
	if !strings.Contains(errOut, "printf: write error:") {
		t.Errorf("stderr = %q, want the wording to name printf", errOut)
	}
}

func TestFailedWriteIsSilentWithoutAWording(t *testing.T) {
	sem, _ := failingWrites()
	// No BuiltinWriteError: the status carries the failure and nothing is
	// said, which is a wording rather than a behavior.
	out, errOut, st := runClosedWrite(t, `echo hi >&-`, sem, Diagnostics{})
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
	if out != "" || errOut != "" {
		t.Errorf("stdout %q, stderr %q — both should be empty", out, errOut)
	}
}

func TestFailedWriteKeptAsSuccessWhereTheAxisSaysNo(t *testing.T) {
	sem, diag := failingWrites()
	sem.BuiltinWriteErrorFailsTheCommand = No
	out, errOut, _ := runClosedWrite(t, `echo hi >&-; echo st=$?`, sem, diag)
	if out != "st=0\n" {
		t.Errorf("stdout = %q, want st=0 — the text is quietly lost", out)
	}
	if errOut != "" {
		t.Errorf("stderr = %q, want nothing said", errOut)
	}
}

func TestFailedWriteRefusedWhereNoDialectAnswered(t *testing.T) {
	// The axis is asked only at the disagreement: `echo hi` on an open
	// stream needs no dialect, and the same echo on a closed one does.
	out, errOut, st := runClosedWrite(t, `echo hi >&-`, Semantics{}, Diagnostics{})
	if st != 2 || !strings.Contains(errOut, "no dialect was chosen") {
		t.Errorf("status %d, stderr %q — an unanswered axis should refuse", st, errOut)
	}
	if out != "" {
		t.Errorf("stdout = %q, want none", out)
	}
}

func TestStreamClosedByExecFailsEachLaterBuiltin(t *testing.T) {
	sem, diag := failingWrites()
	_, errOut, _ := runClosedWrite(t, `exec >&-; echo a; echo st=$? >&2; echo b; echo st=$? >&2`, sem, diag)
	if got := strings.Count(errOut, "echo: write error:"); got != 2 {
		t.Errorf("stderr = %q, want a complaint per builtin, got %d", errOut, got)
	}
	if got := strings.Count(errOut, "st=1"); got != 2 {
		t.Errorf("stderr = %q, want st=1 after each failure", errOut)
	}
}

func TestGroupRedirectFailsEveryWriteInside(t *testing.T) {
	sem, diag := failingWrites()
	out, errOut, _ := runClosedWrite(t, `{ echo a; echo b; } >&-; echo st=$?`, sem, diag)
	if got := strings.Count(errOut, "echo: write error:"); got != 2 {
		t.Errorf("stderr = %q, want both writes reported, got %d", errOut, got)
	}
	if out != "st=1\n" {
		t.Errorf("stdout = %q, want the group to report the failure", out)
	}
}

func TestFailedWriteDoesNotStopTheScript(t *testing.T) {
	sem, diag := failingWrites()
	out, _, _ := runClosedWrite(t, `echo a >&-; echo still-here`, sem, diag)
	if out != "still-here\n" {
		t.Errorf("stdout = %q — the failure is per command, never fatal", out)
	}
}

func TestNothingToWriteCannotFail(t *testing.T) {
	sem, diag := failingWrites()
	out, errOut, _ := runClosedWrite(t, `echo -n "" >&-; echo st=$?; printf "" >&-; echo st=$?`, sem, diag)
	if out != "st=0\nst=0\n" {
		t.Errorf("stdout = %q, want both empty writes to succeed", out)
	}
	if errOut != "" {
		t.Errorf("stderr = %q, want nothing said", errOut)
	}
}

func TestWrappedBuiltinBlamesTheOneThatWrote(t *testing.T) {
	sem, diag := failingWrites()
	// `command echo` writes through a wrapper, and the complaint names the
	// builtin whose write it was rather than the wrapper.
	out, errOut, _ := runClosedWrite(t, `command echo hi >&-; echo st=$?`, sem, diag)
	if out != "st=1\n" {
		t.Errorf("stdout = %q, want st=1", out)
	}
	if !strings.Contains(errOut, "echo: write error:") || strings.Contains(errOut, "command:") {
		t.Errorf("stderr = %q, want echo named and the wrapper not", errOut)
	}
}

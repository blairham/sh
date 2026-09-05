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

// Every builtin that writes reaches the same axis and the same wording, which
// is only true while every builtin's write is *recorded*. These three wrote
// their stream directly and threw the error away, so the axis was never asked
// and the wording never reached: each answered 0 in silence whatever the
// dialect said.
func TestEveryWritingBuiltinReachesTheWriteFailureAxis(t *testing.T) {
	sem, diag := failingWrites()
	sem.ExportListing = DeclareListingCommandWord
	sem.ReadonlyListing = DeclareListingCommandWord
	sem.DeclareValueQuoting = ListingQuoteAlwaysEscaped
	for _, c := range []struct{ name, src, blames string }{
		{"pwd", `pwd >&-; echo st=$?`, "pwd: write error:"},
		{"times", `times >&-; echo st=$?`, "times: write error:"},
		{"export", `export >&-; echo st=$?`, "export: write error:"},
		{"readonly", `readonly RO=1; readonly >&-; echo st=$?`, "readonly: write error:"},
	} {
		out, errOut, _ := runClosedWrite(t, c.src, sem, diag)
		if out != "st=1\n" {
			t.Errorf("%s: stdout = %q, want the failed write to fail the command", c.name, out)
		}
		if !strings.Contains(errOut, c.blames) {
			t.Errorf("%s: stderr = %q, want it to carry %q", c.name, errOut, c.blames)
		}
	}
}

// And the axis still decides: a dialect that keeps a failed write as a success
// keeps these too, which is what says they are on the shared path rather than
// carrying an answer of their own.
func TestTheWritingBuiltinsFollowTheAxisWhenItSaysNo(t *testing.T) {
	sem, diag := failingWrites()
	sem.BuiltinWriteErrorFailsTheCommand = No
	sem.ExportListing = DeclareListingCommandWord
	sem.DeclareValueQuoting = ListingQuoteAlwaysEscaped
	for _, src := range []string{
		`pwd >&-; echo st=$?`,
		`times >&-; echo st=$?`,
		`export >&-; echo st=$?`,
	} {
		out, errOut, _ := runClosedWrite(t, src, sem, diag)
		if out != "st=0\n" {
			t.Errorf("%q: stdout = %q, want st=0", src, out)
		}
		if errOut != "" {
			t.Errorf("%q: stderr = %q, want nothing said", src, errOut)
		}
	}
}

// `export` and `readonly` with no operands list, which is what gives them a
// write to fail in the first place. They wrote nothing at all, so the two
// questions are one change and one test: the listing is there, and it is the
// same listing `-p` asks for.
func TestExportAndReadonlyWithNoOperandsList(t *testing.T) {
	sem, diag := failingWrites()
	sem.ExportListing = DeclareListingCommandWord
	sem.ReadonlyListing = DeclareListingCommandWord
	sem.DeclareValueQuoting = ListingQuoteWhenNeededPlain
	out, _, _ := runClosedWrite(t, `export V=1; readonly R=2; export; echo --; export -p; echo --; readonly; echo --; readonly -p`, sem, diag)
	parts := strings.Split(out, "--\n")
	if len(parts) != 4 {
		t.Fatalf("stdout = %q, want four listings", out)
	}
	if !strings.Contains(parts[0], "export V=1") {
		t.Errorf("bare export listed %q, want the exported name in it", parts[0])
	}
	if parts[0] != parts[1] {
		t.Errorf("bare export listed %q and -p listed %q, want the same listing", parts[0], parts[1])
	}
	if !strings.Contains(parts[2], "readonly R=2") {
		t.Errorf("bare readonly listed %q, want the readonly name in it", parts[2])
	}
	if parts[2] != parts[3] {
		t.Errorf("bare readonly listed %q and -p listed %q, want the same listing", parts[2], parts[3])
	}
}

// An operand is not a listing: `export V=1` sets and says nothing, which is
// the line the bare form has to stop at.
func TestExportWithOperandsDoesNotList(t *testing.T) {
	sem, diag := failingWrites()
	sem.ExportListing = DeclareListingCommandWord
	sem.DeclareValueQuoting = ListingQuoteWhenNeededPlain
	out, errOut, st := runClosedWrite(t, `export V=1; readonly R=2`, sem, diag)
	if out != "" || errOut != "" || st != 0 {
		t.Errorf("stdout %q, stderr %q, status %d — an assignment writes nothing", out, errOut, st)
	}
}

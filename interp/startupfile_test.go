// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// startupRun runs src the way a shell runs its own startup file, and reports
// everything it said along with the status it left.
//
// ReturnOutsideAFunctionIsRefused is set to the answer that *does* refuse, on
// purpose: a startup file must be a place a `return` is obeyed whatever a
// script's top level does, so a test that answered No here would pass for a
// shell that had never been given a frame to return from (#1422).
func startupRun(t *testing.T, src string, carries Answer) (string, int) {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	sem.ReturnOutsideAFunctionIsRefused = Yes
	sem.StartupFileReturnCarriesItsArgument = carries
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "sh"})
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	st, err := r.RunStartupFile(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	return buf.String(), st
}

// A startup file is a sourced script, so a `return` in one is obeyed: it stops
// reading the file and says nothing.
//
// Unanimous across the panel, measured through a pty with
// `echo BEFORE; return 3; echo AFTER` as the whole file — bash 5.3.15, bash
// 3.2.57, bash under argv[0] `sh`, dash, ksh93 and zsh 5.9.2 all print BEFORE,
// stop there, and diagnose nothing. It is how a real `~/.bashrc` bails out
// early, most often as `[ -z "$PS1" ] && return`.
func TestAReturnInAStartupFileStopsTheFileAndSaysNothing(t *testing.T) {
	for _, carries := range []Answer{Yes, No} {
		out, _ := startupRun(t, "echo BEFORE\nreturn 3\necho AFTER\n", carries)
		if !strings.Contains(out, "BEFORE") {
			t.Errorf("carries=%v: said %q, want the lines before the return to have run", carries, out)
		}
		if strings.Contains(out, "AFTER") {
			t.Errorf("carries=%v: said %q, want the file to stop at the return", carries, out)
		}
		if strings.Contains(out, "can only") {
			t.Errorf("carries=%v: said %q, want no diagnostic — no shell in the panel writes one here", carries, out)
		}
	}
}

// What the argument does is where the panel splits, and it splits over the
// argument alone. Measured through a pty, reading `$?` at the first prompt:
//
//	rc              bash  bash32  dash  ksh93  zsh
//	return 3           0       0     3      3    3
//	false; return 3    1       1     3      3    3
//	false; return      1       1     1      1    1
//	(exit 5)           5       5     5      5    5
//
// The last two rows are what make this about the argument: bash does carry a
// startup file's status out, and a `return` with no argument means the last
// command's status everywhere.
func TestTheStatusAStartupFileLeaves(t *testing.T) {
	for _, c := range []struct {
		name         string
		src          string
		carries, not int
	}{
		{"return with an argument", "return 3\n", 3, 0},
		{"a failure, then a return with an argument", "false\nreturn 3\n", 3, 1},
		{"a failure, then a bare return", "false\nreturn\n", 1, 1},
		{"no return at all", "(exit 5)\n", 5, 5},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, st := startupRun(t, c.src, Yes); st != c.carries {
				t.Errorf("carrying the argument: status = %d, want %d", st, c.carries)
			}
			if _, st := startupRun(t, c.src, No); st != c.not {
				t.Errorf("discarding the argument: status = %d, want %d", st, c.not)
			}
		})
	}
}

// The frame is given back, so the file after this one — and the script the
// shell was started for — is not left looking like the inside of a sourced
// file. Without that, reading any startup file at all would make a later
// `return` at a script's own top level legal for good.
func TestAStartupFileDoesNotLeaveTheShellLookingSourced(t *testing.T) {
	var buf strings.Builder
	sem := PosixSemantics()
	sem.ReturnOutsideAFunctionIsRefused = Yes
	sem.StartupFileReturnCarriesItsArgument = No
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "sh"})

	rc, err := syntax.Parse("echo rc\n", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RunStartupFile(context.Background(), rc); err != nil {
		t.Fatal(err)
	}
	script, err := syntax.Parse("return 7\necho \"after st=$?\"\n", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), script); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "rc") {
		t.Fatalf("said %q, want the startup file to have run", out)
	}
	if !strings.Contains(out, "can only `return'") || !strings.Contains(out, "after st=2") {
		t.Errorf("said %q, want the script's own return refused as one with nothing to return from", out)
	}
}

// And the shell is not left in a returning state: a startup file that returns
// is followed by the next startup file and then by the prompt, not skipped
// past. Two files in a row, the first of them returning.
func TestAStartupFileThatReturnsDoesNotStopTheNextOne(t *testing.T) {
	var buf strings.Builder
	sem := PosixSemantics()
	sem.ReturnOutsideAFunctionIsRefused = Yes
	sem.StartupFileReturnCarriesItsArgument = Yes
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "sh"})

	for _, src := range []string{"echo first\nreturn 3\necho no\n", "echo second\n"} {
		f, err := syntax.Parse(src, syntax.Core())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.RunStartupFile(context.Background(), f); err != nil {
			t.Fatal(err)
		}
	}
	out := buf.String()
	for _, want := range []string{"first", "second"} {
		if !strings.Contains(out, want) {
			t.Errorf("said %q, want %q in it", out, want)
		}
	}
	if strings.Contains(out, "no") && !strings.Contains(out, "second") {
		t.Errorf("said %q, want the first file to have stopped at its return", out)
	}
}

// A shell with no answer to the status question refuses by name rather than
// picking one. The file still stops where it was told to — that part is
// unanimous and is not what is unanswered.
func TestAStartupFileReturnWithNoAnswerRecordedIsNamed(t *testing.T) {
	out, _ := startupRun(t, "echo BEFORE\nreturn 3\necho AFTER\n", Unspecified)
	if !strings.Contains(out, "return") || !strings.Contains(out, "disagree") {
		t.Errorf("said %q, want the axis named", out)
	}
	if strings.Contains(out, "AFTER") {
		t.Errorf("said %q, want the file to stop at the return regardless", out)
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// What dash does about `eval` and `.`. It is the POSIX-faithful member of the
// panel and the odd one out on almost every question here, which is why the
// substrate's preset follows it and the other three override.

func runDash(t *testing.T, dir, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, dash.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem, diag := dash.Semantics(), dash.Diagnostics()
	r := &interp.Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &diag,
		Dir: dir, Name: "dash", Vars: map[string]string{"PATH": dir},
	}
	dash.Apply(r)
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return buf.String(), st
}

// TestDashHasNoSource is the reason `source` is a dialect's answer rather than
// a substrate builtin. If the core offered it, dash would have no way to say
// no — and `command -v source` in the real binary reports "not found".
func TestDashHasNoSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.sh")
	if err := os.WriteFile(path, []byte("echo via-source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, st := runDash(t, dir, `source `+path)
	if strings.Contains(out, "via-source") {
		t.Errorf("dash has no `source`: %q", out)
	}
	if st != 127 {
		t.Errorf("status = %d, want 127 for a command that is not there", st)
	}
	// `.` is POSIX and works.
	out, st = runDash(t, dir, `. `+path)
	if st != 0 || strings.TrimSpace(out) != "via-source" {
		t.Errorf("`.` should work: %q status %d", out, st)
	}
}

// TestAnUnparseableEvalIsFatal is dash alone in the panel, and it is the POSIX
// rule the other three abandoned: a special builtin's failure ends a
// non-interactive shell.
func TestAnUnparseableEvalIsFatal(t *testing.T) {
	out, st := runDash(t, t.TempDir(), `eval "if"; echo NOT-REACHED`)
	if strings.Contains(out, "NOT-REACHED") {
		t.Errorf("dash ends the script on an unparseable eval: %q", out)
	}
	if st != 2 {
		t.Errorf("status = %d, want 2", st)
	}
}

// TestDotWithNoOperandIsNotAnError is dash's oddest answer here: the other
// three complain, and dash does nothing and reports success.
func TestDotWithNoOperandIsNotAnError(t *testing.T) {
	if got := dash.Semantics().DotWithNoOperandIsAnError; got != interp.No {
		t.Fatalf("DotWithNoOperandIsAnError = %v, want No", got)
	}
	out, st := runDash(t, t.TempDir(), `. ; echo st=$?`)
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
	if strings.TrimSpace(out) != "st=0" {
		t.Errorf("output = %q, want st=0 and no complaint", out)
	}
}

// TestDotIgnoresArguments is the other dash-only answer: a sourced file still
// sees the caller's positional parameters.
func TestDotIgnoresArguments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.sh")
	if err := os.WriteFile(path, []byte("echo got=$1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ := runDash(t, dir, `set -- OUTER; . `+path+` INNER`)
	if strings.TrimSpace(out) != "got=OUTER" {
		t.Errorf("output = %q, want got=OUTER: dash ignores the words after the file", out)
	}
}

// TestDotHasTwoMessages is why DotNotFound exists. dash says "not found" for a
// bare name PATH did not have and "cannot open …" for a path, where the other
// three use one message for both.
func TestDotHasTwoMessages(t *testing.T) {
	d := dash.Diagnostics()
	if d.DotNotFound == "" {
		t.Fatal("dash needs a separate not-found wording")
	}

	dir := t.TempDir()
	out, _ := runDash(t, dir, `. nosuchname.sh`)
	if !strings.Contains(out, "not found") {
		t.Errorf("a bare name off PATH: %q, want \"not found\"", out)
	}

	out, _ = runDash(t, dir, `. `+filepath.Join(dir, "absent.sh"))
	if !strings.Contains(out, "cannot open") {
		t.Errorf("a path that will not open: %q, want \"cannot open\"", out)
	}
	// And dash truncates strerror for that one, which is spelled out in the
	// format rather than taken from the error.
	if strings.Contains(out, "No such file or directory") {
		t.Errorf("output = %q: dash says \"No such file\" here, not the full strerror", out)
	}
}

// TestExecAxes records what dash does about `exec`, where it keeps the POSIX
// answer on one axis and is the odd one out on the other.
func TestExecAxes(t *testing.T) {
	s := dash.Semantics()
	// With bash, and against ksh93 and zsh.
	if got := s.ExecFailureRunsExitTrap; got != interp.Yes {
		t.Errorf("ExecFailureRunsExitTrap = %v, want Yes", got)
	}
	// Alone: `exec -a name cmd` is a command called "-a" here.
	if got := s.ExecTakesOptions; got != interp.No {
		t.Errorf("ExecTakesOptions = %v, want No", got)
	}
	out, _ := runDash(t, t.TempDir(), `exec -a myname echo hi`)
	if strings.Contains(out, "hi") {
		t.Errorf("dash does not read options here: %q", out)
	}
	if !strings.Contains(out, "-a") {
		t.Errorf("output = %q, want -a reported as the command", out)
	}

	// dash hands a directory to execve rather than checking first.
	if dash.Diagnostics().DirectoryReason == "" {
		t.Error("dash reports execve's own error for a directory")
	}
}

// TestDoubleEqualInTest: dash has only `=`, so `==` is not an operator and the three words are a malformed expression — 2 for both pairs of operands, where the others answer 0 and 1.
func TestDoubleEqualInTest(t *testing.T) {
	dir := t.TempDir()
	if _, st := runDash(t, dir, `test a == a`); st != 2 {
		t.Errorf("equal operands: status %d, want 2", st)
	}
	if _, st := runDash(t, dir, `test a == b`); st != 2 {
		t.Errorf("unequal operands: status %d, want 2", st)
	}
	// The single `=` is unanimous, and pins the difference to the operator
	// rather than to anything else about how the words are read.
	if _, st := runDash(t, dir, `test a = a`); st != 0 {
		t.Errorf("single equals: status %d, want 0", st)
	}
}

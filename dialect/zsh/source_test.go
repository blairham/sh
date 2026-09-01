// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

func runZsh(t *testing.T, dir, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, zsh.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem, diag := zsh.Semantics(), zsh.Diagnostics()
	r := &interp.Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &diag,
		Dir: dir, Name: "zsh", Vars: map[string]string{"PATH": dir},
	}
	zsh.Apply(r)
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return buf.String(), st
}

func TestSourceIsASynonymForDot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.sh")
	if err := os.WriteFile(path, []byte("echo via-source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, st := runZsh(t, dir, `source `+path)
	if st != 0 || strings.TrimSpace(out) != "via-source" {
		t.Errorf("output = %q status %d, want via-source", out, st)
	}
}

// TestASyntaxErrorDependsOnWhereItWasRead is zsh's alone in the panel, and the
// only reason SourcedSyntaxErrorStatus exists: the same unparseable text is 1
// from -c and 126 from a file `.` opened. bash says 2 for both and ksh93 3 for
// both, so folding the two together would have lost one of zsh's answers.
func TestASyntaxErrorDependsOnWhereItWasRead(t *testing.T) {
	d := zsh.Diagnostics()
	if d.SyntaxErrorStatus != 1 {
		t.Errorf("SyntaxErrorStatus = %d, want 1", d.SyntaxErrorStatus)
	}
	if d.SourcedSyntaxErrorStatus != 126 {
		t.Errorf("SourcedSyntaxErrorStatus = %d, want 126", d.SourcedSyntaxErrorStatus)
	}

	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.sh")
	if err := os.WriteFile(bad, []byte("if\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, st := runZsh(t, dir, `eval "if"`); st != 1 {
		t.Errorf("eval of unparseable text = %d, want 1", st)
	}
	if _, st := runZsh(t, dir, `. `+bad); st != 126 {
		t.Errorf("sourcing unparseable text = %d, want 126", st)
	}
}

func TestEvalAndDotAxes(t *testing.T) {
	s := zsh.Semantics()
	for _, tc := range []struct {
		axis string
		got  interp.Answer
		want interp.Answer
	}{
		{"BuiltinSyntaxErrorFatal", s.BuiltinSyntaxErrorFatal, interp.No},
		{"DotMissingFileFatal", s.DotMissingFileFatal, interp.No},
		{"DotPassesArguments", s.DotPassesArguments, interp.Yes},
		// Not bash: PATH missing the file is the end of it here.
		{"DotFallsBackToCurrentDirectory", s.DotFallsBackToCurrentDirectory, interp.No},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.axis, tc.got, tc.want)
		}
	}
	// 127 for a file it cannot open, where bash says 1 — the most divergent
	// answer in the panel, and it survives rather than ending the script.
	if got := zsh.Diagnostics().DotCannotOpenStatus; got != 127 {
		t.Errorf("DotCannotOpenStatus = %d, want 127", got)
	}
}

// TestExecAxesAndWording records what zsh does about `exec`, which is the most
// divergent of the four.
func TestExecAxesAndWording(t *testing.T) {
	s := zsh.Semantics()
	// zsh and ksh93 drop the EXIT trap where dash and bash run it.
	if got := s.ExecFailureRunsExitTrap; got != interp.No {
		t.Errorf("ExecFailureRunsExitTrap = %v, want No", got)
	}
	if got := s.ExecTakesOptions; got != interp.Yes {
		t.Errorf("ExecTakesOptions = %v, want Yes", got)
	}
	d := zsh.Diagnostics()
	// The only dialect that lowercases every strerror string it quotes.
	if !d.LowercaseReason {
		t.Error("LowercaseReason should be true for zsh")
	}
	// zsh hands the path to execve rather than checking for a directory, so it
	// reports the permission error that comes back.
	if d.DirectoryReason == "" {
		t.Error("zsh reports execve's own error for a directory")
	}
	if d.NamesResolvedPath {
		t.Error("zsh reports the operand as written, not resolved")
	}
}

// TestAFailedExecDropsTheExitTrap is the axis as behavior.
func TestAFailedExecDropsTheExitTrap(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `trap "echo TRAP" EXIT; exec nosuchcmd-xyz`)
	if strings.Contains(out, "TRAP") {
		t.Errorf("zsh drops the EXIT trap after a failed exec: %q", out)
	}
	if st != 127 {
		t.Errorf("status = %d, want 127", st)
	}
}

// TestDoubleEqualInTest: zsh takes `==` as a second spelling of `=`, and is
// the one dialect where it has to be quoted to reach the builtin at all — an
// unquoted word starting with `=` is expanded to the path of the command
// named after it, so bare `==` is a search for a command called `=`. Real zsh
// says "= not found" and exits 1, and so do we.
func TestDoubleEqualInTest(t *testing.T) {
	dir := t.TempDir()
	if _, st := runZsh(t, dir, `test a "==" a`); st != 0 {
		t.Errorf("equal operands: status %d, want 0", st)
	}
	if _, st := runZsh(t, dir, `test a "==" b`); st != 1 {
		t.Errorf("unequal operands: status %d, want 1", st)
	}
	// The single `=` is unanimous, and pins the difference to the operator
	// rather than to anything else about how the words are read.
	if _, st := runZsh(t, dir, `test a = a`); st != 0 {
		t.Errorf("single equals: status %d, want 0", st)
	}
	// Unquoted, the expansion gets it first and the builtin never sees it.
	if out, st := runZsh(t, dir, `test a == a`); st != 1 || !strings.Contains(out, "not found") {
		t.Errorf("unquoted: %q status %d, want the `=` expansion to miss", out, st)
	}
}

// TestABracketNamesItself: `[` is `test` under another name, and the
// diagnostic blames the name that was typed. Run rather than asserted against
// the wording string, since the wording is what would be wrong.
func TestABracketNamesItself(t *testing.T) {
	dir := t.TempDir()
	if out, _ := runZsh(t, dir, `test 1 -eq a`); !strings.Contains(out, `:test:`) {
		t.Errorf("test: said %q, want %q", out, `:test:`)
	}
	if out, _ := runZsh(t, dir, `[ 1 -eq a ]`); !strings.Contains(out, `:[:`) {
		t.Errorf("bracket: said %q, want %q", out, `:[:`)
	}
	// The unary wording is a separate string and carries the name too.
	if out, _ := runZsh(t, dir, `[ -Q x ]`); !strings.Contains(out, `:[:`) {
		t.Errorf("unary bracket: said %q, want %q", out, `:[:`)
	}
}

// TestAKilledCommandsStatus: 128 + 13.
func TestAKilledCommandsStatus(t *testing.T) {
	dir := t.TempDir()
	// PATH is the temp directory alone, so the command that dies has to be
	// made here rather than borrowed from the host.
	exe := filepath.Join(dir, "selfkill")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nkill -PIPE $$\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, _ := runZsh(t, dir, "selfkill\necho st=$?\n")
	if !strings.Contains(out, "st=141") {
		t.Errorf("said %q, want st=141", out)
	}
}

// TestPipefail: zsh has the option.
func TestPipefail(t *testing.T) {
	dir := t.TempDir()
	out, _ := runZsh(t, dir, "set -o pipefail\n(exit 3) | (exit 4) | true\necho st=$?\n")
	if !strings.Contains(out, "st=4") {
		t.Errorf("said %q, want st=4", out)
	}
}

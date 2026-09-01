// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// What bash does about `eval` and `.`, measured against the real binary and
// recorded here rather than in the substrate.

func runBash(t *testing.T, dir, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, bash.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem, diag := bash.Semantics(), bash.Diagnostics()
	r := &interp.Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &diag,
		Dir: dir, Name: "bash", Vars: map[string]string{"PATH": dir},
	}
	bash.Apply(r)
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
	out, st := runBash(t, dir, `source `+path)
	if st != 0 || strings.TrimSpace(out) != "via-source" {
		t.Errorf("output = %q status %d, want via-source", out, st)
	}
}

func TestEvalAndDotAxes(t *testing.T) {
	s := bash.Semantics()
	for _, tc := range []struct {
		axis string
		got  interp.Answer
		want interp.Answer
	}{
		// Measured: `eval "if"; echo REACHED` prints REACHED here and does
		// not in dash.
		{"BuiltinSyntaxErrorFatal", s.BuiltinSyntaxErrorFatal, interp.No},
		{"DotMissingFileFatal", s.DotMissingFileFatal, interp.No},
		{"DotPassesArguments", s.DotPassesArguments, interp.Yes},
		// The one shell in the panel that does this.
		{"DotFallsBackToCurrentDirectory", s.DotFallsBackToCurrentDirectory, interp.Yes},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.axis, tc.got, tc.want)
		}
	}
}

// TestAnUnparseableEvalIsSurvivable is the axis as behavior rather than as a
// field, which is what stops the two drifting apart.
func TestAnUnparseableEvalIsSurvivable(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `eval "if"; echo REACHED`)
	if !strings.Contains(out, "REACHED") {
		t.Errorf("bash reports an unparseable eval and carries on: %q", out)
	}
}

// TestDotFindsAFileInTheCurrentDirectory is bash's alone. PATH here has the
// temp directory in it, so the file is found without the fallback; the point
// of the second half is that the fallback is what finds it when PATH cannot.
func TestDotFindsAFileInTheCurrentDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fb.sh"), []byte("echo cwd-hit\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	f, err := syntax.Parse(`. fb.sh`, bash.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	sem, diag := bash.Semantics(), bash.Diagnostics()
	r := &interp.Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &diag,
		Dir: dir, Name: "bash",
		// PATH deliberately cannot reach it, so only the fallback can.
		Vars: map[string]string{"PATH": t.TempDir()},
	}
	bash.Apply(r)
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "cwd-hit") {
		t.Errorf("bash looks in the current directory once PATH misses: %q", buf.String())
	}
}

func TestDotStatusesAreBashs(t *testing.T) {
	d := bash.Diagnostics()
	// Two numbers for what reads like one failure, which is why they are two
	// fields: a file it cannot open is 1 and a missing operand is 2.
	if got := d.DotCannotOpenStatus; got != 1 {
		t.Errorf("DotCannotOpenStatus = %d, want 1", got)
	}
	if got := d.DotNoOperandStatus; got != 2 {
		t.Errorf("DotNoOperandStatus = %d, want 2", got)
	}
	// bash prints a complaint and a usage line, and only the first carries the
	// shell's prefix — so the wording itself has to hold the newline.
	if !strings.Contains(d.DotNoOperand, "\n") {
		t.Errorf("DotNoOperand = %q, want the two lines bash prints", d.DotNoOperand)
	}
	// A syntax error is 2 whether it was read from -c or from a sourced file,
	// so the sourced override stays empty here. zsh is the one that needs it.
	if got := d.SourcedSyntaxErrorStatus; got != 0 {
		t.Errorf("SourcedSyntaxErrorStatus = %d, want 0: bash answers both the same", got)
	}
}

// TestExecAxesAndWording records what bash does about `exec`.
func TestExecAxesAndWording(t *testing.T) {
	s := bash.Semantics()
	if got := s.ExecFailureRunsExitTrap; got != interp.Yes {
		t.Errorf("ExecFailureRunsExitTrap = %v, want Yes", got)
	}
	if got := s.ExecTakesOptions; got != interp.Yes {
		t.Errorf("ExecTakesOptions = %v, want Yes", got)
	}
	d := bash.Diagnostics()
	// bash alone names the path it tried rather than the operand as written,
	// and only for `exec` — the same bash reports `. ./nosuch.sh` as written.
	if !d.NamesResolvedPath {
		t.Error("NamesResolvedPath should be true for bash")
	}
	// It checks for a directory itself rather than reporting execve's EACCES,
	// so it needs no override for that reason.
	if d.DirectoryReason != "" {
		t.Errorf("DirectoryReason = %q, want empty: bash says what the OS said", d.DirectoryReason)
	}
	if d.LowercaseReason {
		t.Error("bash prints the C strerror string as it comes")
	}
}

// TestAFailedExecRunsTheExitTrap is the axis as behavior. bash and dash run
// it; ksh93 and zsh drop it.
func TestAFailedExecRunsTheExitTrap(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `trap "echo TRAP" EXIT; exec nosuchcmd-xyz`)
	if !strings.Contains(out, "TRAP") {
		t.Errorf("bash runs the EXIT trap after a failed exec: %q", out)
	}
	if st != 127 {
		t.Errorf("status = %d, want 127", st)
	}
}

// TestDoubleEqualInTest: bash takes `==` as a second spelling of `=`.
func TestDoubleEqualInTest(t *testing.T) {
	dir := t.TempDir()
	if _, st := runBash(t, dir, `test a == a`); st != 0 {
		t.Errorf("equal operands: status %d, want 0", st)
	}
	if _, st := runBash(t, dir, `test a == b`); st != 1 {
		t.Errorf("unequal operands: status %d, want 1", st)
	}
	// The single `=` is unanimous, and pins the difference to the operator
	// rather than to anything else about how the words are read.
	if _, st := runBash(t, dir, `test a = a`); st != 0 {
		t.Errorf("single equals: status %d, want 0", st)
	}
}

// TestABracketNamesItself: `[` is `test` under another name, and the
// diagnostic blames the name that was typed. Run rather than asserted against
// the wording string, since the wording is what would be wrong.
func TestABracketNamesItself(t *testing.T) {
	dir := t.TempDir()
	if out, _ := runBash(t, dir, `test a b c`); !strings.Contains(out, `test: b: binary operator expected`) {
		t.Errorf("test: said %q, want %q", out, `test: b: binary operator expected`)
	}
	if out, _ := runBash(t, dir, `[ a b c ]`); !strings.Contains(out, `[: b: binary operator expected`) {
		t.Errorf("bracket: said %q, want %q", out, `[: b: binary operator expected`)
	}
	// The unary wording is a separate string and carries the name too.
	if out, _ := runBash(t, dir, `[ -Q x ]`); !strings.Contains(out, `[: -Q: unary operator expected`) {
		t.Errorf("unary bracket: said %q, want %q", out, `[: -Q: unary operator expected`)
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
	out, _ := runBash(t, dir, "selfkill\necho st=$?\n")
	if !strings.Contains(out, "st=141") {
		t.Errorf("said %q, want st=141", out)
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

func runKsh(t *testing.T, dir, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, ksh.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem, diag := ksh.Semantics(), ksh.Diagnostics()
	r := &interp.Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &diag,
		Dir: dir, Name: "ksh", Vars: map[string]string{"PATH": dir},
	}
	ksh.Apply(r)
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return buf.String(), st
}

// TestApplyAddsSourceAndStillRemovesLocal covers both halves of Apply, because
// the addition arrived later and the removal is easy to lose to it.
func TestApplyAddsSourceAndStillRemovesLocal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.sh")
	if err := os.WriteFile(path, []byte("echo via-source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, st := runKsh(t, dir, `source `+path)
	if st != 0 || strings.TrimSpace(out) != "via-source" {
		t.Errorf("`source` should work: %q status %d", out, st)
	}

	out, st = runKsh(t, dir, `f() { local v=in; }; f`)
	if !strings.Contains(out, "local: not found") {
		t.Errorf("ksh93 still has no `local`: %q", out)
	}
	if st != 127 {
		t.Errorf("status = %d, want 127", st)
	}
}

// TestTheTwoHalvesOfTheFatalRuleDisagree is why one axis was not enough.
//
// POSIX makes any special builtin's failure fatal. ksh93 kept half of it: a
// file `.` cannot open ends the script, and unparseable text handed to `eval`
// does not. dash kept both halves, bash and zsh neither.
func TestTheTwoHalvesOfTheFatalRuleDisagree(t *testing.T) {
	s := ksh.Semantics()
	if got := s.BuiltinSyntaxErrorFatal; got != interp.No {
		t.Errorf("BuiltinSyntaxErrorFatal = %v, want No", got)
	}
	if got := s.DotMissingFileFatal; got != interp.Yes {
		t.Errorf("DotMissingFileFatal = %v, want Yes", got)
	}

	dir := t.TempDir()
	// `$?` is read immediately, because the trailing echo succeeds and would
	// otherwise be the status this asserts on — the script exits 0 here in the
	// real shell too, and the 3 belongs to the eval.
	out, _ := runKsh(t, dir, `eval "if"; echo REACHED st=$?`)
	if !strings.Contains(out, "REACHED") {
		t.Errorf("an unparseable eval is survivable here: %q", out)
	}
	if !strings.Contains(out, "st=3") {
		t.Errorf("output = %q, want st=3 — ksh93's syntax status", out)
	}

	out, st := runKsh(t, dir, `. `+filepath.Join(dir, "absent.sh")+`; echo NOT-REACHED`)
	if strings.Contains(out, "NOT-REACHED") {
		t.Errorf("a file `.` cannot open ends the script here: %q", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
}

// TestNoOperandIsFatalWithItsOwnStatus is the case that caught a bug: the
// generic fatal status for ksh93 is 1, and this failure reports 2. Taking the
// status from the fatal path rather than from the field was wrong by one.
func TestNoOperandIsFatalWithItsOwnStatus(t *testing.T) {
	if got := ksh.Diagnostics().DotNoOperandStatus; got != 2 {
		t.Fatalf("DotNoOperandStatus = %d, want 2", got)
	}
	out, st := runKsh(t, t.TempDir(), `. ; echo NOT-REACHED`)
	if strings.Contains(out, "NOT-REACHED") {
		t.Errorf("`.` with no operand ends the script here: %q", out)
	}
	if st != 2 {
		t.Errorf("status = %d, want 2 — not the generic fatal status of 1", st)
	}
}

func TestDotPassesArgumentsHere(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.sh")
	if err := os.WriteFile(path, []byte("echo got=$1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ := runKsh(t, dir, `set -- OUTER; . `+path+` INNER; echo after=$1`)
	if got := strings.TrimSpace(out); got != "got=INNER\nafter=OUTER" {
		t.Errorf("output = %q, want the file to see INNER and the caller OUTER", got)
	}
}

// TestExecAxes records what ksh93 does about `exec`.
func TestExecAxes(t *testing.T) {
	s := ksh.Semantics()
	if got := s.ExecFailureRunsExitTrap; got != interp.No {
		t.Errorf("ExecFailureRunsExitTrap = %v, want No", got)
	}
	if got := s.ExecTakesOptions; got != interp.Yes {
		t.Errorf("ExecTakesOptions = %v, want Yes", got)
	}
	out, st := runKsh(t, t.TempDir(), `trap "echo TRAP" EXIT; exec nosuchcmd-xyz`)
	if strings.Contains(out, "TRAP") {
		t.Errorf("ksh93 drops the EXIT trap after a failed exec: %q", out)
	}
	if st != 127 {
		t.Errorf("status = %d, want 127", st)
	}
}

// TestDoubleEqualInTest: ksh93 takes `==` as a second spelling of `=`.
func TestDoubleEqualInTest(t *testing.T) {
	dir := t.TempDir()
	if _, st := runKsh(t, dir, `test a == a`); st != 0 {
		t.Errorf("equal operands: status %d, want 0", st)
	}
	if _, st := runKsh(t, dir, `test a == b`); st != 1 {
		t.Errorf("unequal operands: status %d, want 1", st)
	}
	// The single `=` is unanimous, and pins the difference to the operator
	// rather than to anything else about how the words are read.
	if _, st := runKsh(t, dir, `test a = a`); st != 0 {
		t.Errorf("single equals: status %d, want 0", st)
	}
}

// TestABracketNamesItself: `[` is `test` under another name, and the
// diagnostic blames the name that was typed. Run rather than asserted against
// the wording string, since the wording is what would be wrong.
func TestABracketNamesItself(t *testing.T) {
	dir := t.TempDir()
	if out, _ := runKsh(t, dir, `test a b c`); !strings.Contains(out, `test: b: unknown operator`) {
		t.Errorf("test: said %q, want %q", out, `test: b: unknown operator`)
	}
	if out, _ := runKsh(t, dir, `[ a b c ]`); !strings.Contains(out, `[: b: unknown operator`) {
		t.Errorf("bracket: said %q, want %q", out, `[: b: unknown operator`)
	}
	// The unary wording is a separate string and carries the name too.
	if out, _ := runKsh(t, dir, `[ -Q x ]`); !strings.Contains(out, `[: -Q: unknown operator`) {
		t.Errorf("unary bracket: said %q, want %q", out, `[: -Q: unknown operator`)
	}
}

// TestAKilledCommandsStatus: ksh93 counts from 256, so 256 + 13 — measured across eight signals, not a special case for one.
func TestAKilledCommandsStatus(t *testing.T) {
	dir := t.TempDir()
	// PATH is the temp directory alone, so the command that dies has to be
	// made here rather than borrowed from the host.
	exe := filepath.Join(dir, "selfkill")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nkill -PIPE $$\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, _ := runKsh(t, dir, "selfkill\necho st=$?\n")
	if !strings.Contains(out, "st=269") {
		t.Errorf("said %q, want st=269", out)
	}
}

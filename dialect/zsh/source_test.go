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

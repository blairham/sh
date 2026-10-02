// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package zsh_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// TestExecHandsItsStandardOutputToTheReplacement pins `exec cmd` giving the
// command the shell's standard output. `exec` is a builtin, and this dialect
// holds a builtin's standard output until it returns, so the stream the
// replacement was built from was the hold, which has no descriptor: the
// command started with 1 closed and `zsh -fc 'exec /bin/echo hi'` said
// `echo: fflush: Bad file descriptor` (#5434). What the builtin wrote first
// reaches the file before the replacement does.
func TestExecHandsItsStandardOutputToTheReplacement(t *testing.T) {
	dir := t.TempDir()
	out, err := os.Create(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = out.Close() }()
	src := "print -n first; exec /bin/echo hi"
	f, err := syntax.Parse(src, zsh.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	sem, diag := zsh.Semantics(), zsh.Diagnostics()
	var handed []*os.File
	r := &interp.Runner{
		Stdout: out, Stderr: os.Stderr, Semantics: &sem, Diagnostics: &diag,
		Dir: dir, Name: "zsh", Route: interp.RouteScriptFile,
		Vars:    map[string]string{"PATH": "/usr/bin:/bin"},
		Dialect: presetDialect(),
		ReplaceProcess: func(_ string, _, _ []string, files []*os.File) error {
			handed = files
			return os.ErrPermission
		},
	}
	zsh.Apply(r)
	_, _ = r.Run(context.Background(), f)
	if len(handed) < 2 || handed[1] != out {
		t.Fatalf("the replacement was handed %v for standard output, want the shell's own %v", handed, out)
	}
	if b, _ := os.ReadFile(out.Name()); string(b) != "first" {
		t.Errorf("before the replacement the file held %q, want %q", b, "first")
	}
}

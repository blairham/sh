// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package zsh_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// TestTheLastCommandOfACommandStringReplacesTheShell pins which command of a
// `-c` string the shell becomes rather than forks, per interp.TailExec.
// Measured 2026-10-03 on zsh 5.9.2, with P a program that prints its parent's
// pid beside the shell's own `$$`. The hook here records the replacement
// and refuses it, so the program is then forked as it would be anyway.
func TestTheLastCommandOfACommandStringReplacesTheShell(t *testing.T) {
	for _, tc := range []struct {
		src      string
		replaced bool
	}{
		{"P", true},
		{"true; P", true},
		{"x=1 P", true},
		{"true && P", true},
		{"if true; then P; fi", true},
		{"{ P; }", true},
		{"case a in a) P;; esac", true},
		{"P >/dev/null", true},
		{"( P )", true},
		{"( ( P ) )", true},
		{"for i in 1; do P; done", true},
		{"P; true", false},
		{"( P; true )", false},
		{"trap 'echo t' EXIT; P", false},
		{"f() { P; }; f", false},
		{"eval P", false},
		{"! P", false},
		{"P | cat", false},
	} {
		src := strings.ReplaceAll(tc.src, "P", "/bin/sh -c :")
		f, err := syntax.Parse(src, zsh.Dialect())
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		sem, diag := zsh.Semantics(), zsh.Diagnostics()
		dir := t.TempDir()
		var replaced, inDir string
		r := &interp.Runner{
			Stdout: os.Stdout, Stderr: os.Stderr, Semantics: &sem, Diagnostics: &diag,
			Dir: dir, Name: "zsh", Route: interp.RouteCommandString,
			Vars:    map[string]string{"PATH": "/usr/bin:/bin"},
			Dialect: presetDialect(),
			ReplaceProcess: func(d, path string, _, _ []string, _ []*os.File) error {
				replaced, inDir = path, d
				return errors.New("recorded rather than replaced")
			},
		}
		zsh.Apply(r)
		r.LastPart = true
		_, _ = r.Run(context.Background(), f)
		if got := replaced != ""; got != tc.replaced {
			t.Errorf("%s: replaced = %v, want %v", tc.src, got, tc.replaced)
		}
		if replaced != "" && inDir != dir {
			t.Errorf("%s: the replacement starts in %q, want the shell's %q", tc.src, inDir, dir)
		}
	}
}

// TestTheReplacementStartsWhereTheShellIs pins the two things the program a
// shell becomes is handed that a forked one is not: the shell's directory,
// moved to by the hook because a Runner's `cd` never moved the process, and
// `$SHLVL` with this shell taken back out of the count. Measured 2026-10-03
// on zsh 5.9.2: `cd /; exec pwd` prints `/`, and `SHLVL=1 zsh -c 'zsh -c
// "echo \$SHLVL"'` prints 2.
func TestTheReplacementStartsWhereTheShellIs(t *testing.T) {
	f, err := syntax.Parse("cd /; SHLVL=4; /bin/sh -c :", zsh.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	sem, diag := zsh.Semantics(), zsh.Diagnostics()
	var inDir, level string
	r := &interp.Runner{
		Stdout: os.Stdout, Stderr: os.Stderr, Semantics: &sem, Diagnostics: &diag,
		Dir: t.TempDir(), Name: "zsh", Route: interp.RouteCommandString,
		Vars:    map[string]string{"PATH": "/usr/bin:/bin"},
		Dialect: presetDialect(),
		ReplaceProcess: func(d, _ string, _, env []string, _ []*os.File) error {
			inDir = d
			for _, kv := range env {
				if v, ok := strings.CutPrefix(kv, "SHLVL="); ok {
					level = v
				}
			}
			return errors.New("recorded rather than replaced")
		},
	}
	zsh.Apply(r)
	r.LastPart = true
	_, _ = r.Run(context.Background(), f)
	if inDir != "/" {
		t.Errorf("the replacement starts in %q, want /", inDir)
	}
	if level != "3" {
		t.Errorf("the replacement is handed SHLVL=%q, want 3", level)
	}
}

// TestAScriptWithNoInterpreterLineIsNotReplacedInto pins that a last command
// the kernel would not start by itself, an executable script with no `#!`,
// is not handed to the replacement. It runs through this shell as it does
// when it is forked, and it does run.
func TestAScriptWithNoInterpreterLineIsNotReplacedInto(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/ns", []byte("echo ran\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := syntax.Parse("./ns", zsh.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	sem, diag := zsh.Semantics(), zsh.Diagnostics()
	var out strings.Builder
	replaced := false
	r := &interp.Runner{
		Stdout: &out, Stderr: &out, Semantics: &sem, Diagnostics: &diag,
		Dir: dir, Name: "zsh", Route: interp.RouteCommandString,
		Vars:    map[string]string{"PATH": "/usr/bin:/bin"},
		Dialect: presetDialect(),
		ReplaceProcess: func(_, _ string, _, _ []string, _ []*os.File) error {
			replaced = true
			return errors.New("recorded rather than replaced")
		},
	}
	zsh.Apply(r)
	r.LastPart = true
	_, _ = r.Run(context.Background(), f)
	if replaced {
		t.Error("a script with no interpreter line was handed to the replacement")
	}
	if out.String() != "ran\n" {
		t.Errorf("it printed %q, want %q", out.String(), "ran\n")
	}
}

// TestAShellWritingToABufferForksItsLastCommand pins that a shell whose
// standard output is not a file does not replace itself: a replacement is
// handed descriptors by number, and a buffer has none, so the program's
// output would be lost where a forked child has it copied through a pipe.
func TestAShellWritingToABufferForksItsLastCommand(t *testing.T) {
	f, err := syntax.Parse("/bin/echo hi", zsh.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	sem, diag := zsh.Semantics(), zsh.Diagnostics()
	var out strings.Builder
	replaced := false
	r := &interp.Runner{
		Stdout: &out, Stderr: os.Stderr, Semantics: &sem, Diagnostics: &diag,
		Dir: t.TempDir(), Name: "zsh", Route: interp.RouteCommandString,
		Vars:    map[string]string{"PATH": "/usr/bin:/bin"},
		Dialect: presetDialect(),
		ReplaceProcess: func(_, _ string, _, _ []string, _ []*os.File) error {
			replaced = true
			return errors.New("recorded rather than replaced")
		},
	}
	zsh.Apply(r)
	r.LastPart = true
	_, _ = r.Run(context.Background(), f)
	if replaced || out.String() != "hi\n" {
		t.Errorf("replaced = %v and the buffer holds %q, want a fork that wrote %q", replaced, out.String(), "hi\n")
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package bash_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// TestTheLastCommandOfACommandStringReplacesTheShell pins which command of a
// `-c` string the shell becomes rather than forks, per interp.TailExec.
// Measured 2026-10-03 on bash 5.3.20, with P a program that prints its parent's
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
		{"true && P", false},
		{"if true; then P; fi", false},
		{"{ P; }", false},
		{"P >/dev/null", false},
		{"P; true", false},
		{"trap 'echo t' EXIT; P", false},
	} {
		src := strings.ReplaceAll(tc.src, "P", "/bin/sh -c :")
		f, err := syntax.Parse(src, bash.Dialect())
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		sem, diag := bash.Semantics(), bash.Diagnostics()
		dir := t.TempDir()
		var replaced, inDir string
		r := &interp.Runner{
			Stdin: tailDevNull(t), Stdout: os.Stdout, Stderr: os.Stderr, Semantics: &sem, Diagnostics: &diag,
			Dir: dir, Name: "bash", Route: interp.RouteCommandString,
			Vars:    map[string]string{"PATH": "/usr/bin:/bin"},
			Dialect: presetDialect(),
			ReplaceProcess: func(d, path string, _, _ []string, _ []*os.File) error {
				replaced, inDir = path, d
				return errors.New("recorded rather than replaced")
			},
		}
		bash.Apply(r)
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

// tailDevNull is a standard input that is a file, as a shell's always is,
// so the replacement has a number to hand over for it.
func tailDevNull(t *testing.T) *os.File {
	t.Helper()
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

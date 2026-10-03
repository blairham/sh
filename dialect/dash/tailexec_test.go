// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package dash_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// TestTheLastCommandOfACommandStringReplacesTheShell pins which command of a
// `-c` string the shell becomes rather than forks, per interp.TailExec.
// Measured 2026-10-03 on dash 0.5.12, with P a program that prints its parent's
// pid beside the shell's own `$$`. The hook here records the replacement
// and refuses it, so the program is then forked as it would be anyway.
func TestTheLastCommandOfACommandStringReplacesTheShell(t *testing.T) {
	for _, tc := range []struct {
		src      string
		replaced bool
	}{
		{"P", true},
		{"true && P", true},
		{"if true; then P; fi", true},
		{"{ P; }", true},
		{"P >/dev/null", true},
		{"for i in 1; do P; done", false},
		{"P; true", false},
		{"trap 'echo t' EXIT; P", false},
	} {
		src := strings.ReplaceAll(tc.src, "P", "/bin/sh -c :")
		f, err := syntax.Parse(src, dash.Dialect())
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		sem, diag := dash.Semantics(), dash.Diagnostics()
		dir := t.TempDir()
		var replaced, inDir string
		r := &interp.Runner{
			Stdout: os.Stdout, Stderr: os.Stderr, Semantics: &sem, Diagnostics: &diag,
			Dir: dir, Name: "dash", Route: interp.RouteCommandString,
			Vars:    map[string]string{"PATH": "/usr/bin:/bin"},
			Dialect: presetDialect(),
			ReplaceProcess: func(d, path string, _, _ []string, _ []*os.File) error {
				replaced, inDir = path, d
				return errors.New("recorded rather than replaced")
			},
		}
		dash.Apply(r)
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

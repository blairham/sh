// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package ksh_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// TestTheTailIsReachedEvenWhereItCannotBeBecome pins Runner.TailReached: the
// last command of a `-c` string is the program ksh93 becomes, and where it
// cannot become it — nothing by that name, a file that will not start, a file
// with no `#!` — it found that out in the process it was replacing, so the
// end of its input does not follow. Measured 2026-10-15 on ksh93u+ 2012-08-01
// through the newline its `-i -c` writes when its input ends (#6654): the
// diagnostic and no newline for each of the first three rows, the diagnostic
// and the newline for each of the rest. The hook here records the replacement
// and refuses it.
func TestTheTailIsReachedEvenWhereItCannotBeBecome(t *testing.T) {
	dir := t.TempDir()
	for name, text := range map[string]string{"noexec": "echo x\n", "noshebang": "echo x\n"} {
		mode := os.FileMode(0o755)
		if name == "noexec" {
			mode = 0o644
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), mode); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		src     string
		reached bool
	}{
		{"nosuch_tr", true},
		{"true; nosuch_tr", true},
		{"{ nosuch_tr; }", true},
		{"./noexec", true},
		{"./noshebang", true},
		{"/bin/sh -c :", true},
		// The other half of each pair: the same name, not the last thing run.
		{"nosuch_tr; :", false},
		{"nosuch_tr || :", false},
		{"trap : EXIT; nosuch_tr", false},
		{"true", false},
	} {
		f, err := syntax.Parse(tc.src, ksh.Dialect())
		if err != nil {
			t.Fatalf("%s: %v", tc.src, err)
		}
		sem, diag := ksh.Semantics(), ksh.Diagnostics()
		// Files on every stream, as a shell's always are, so the replacement
		// has a number to hand over for each.
		null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		r := (&interp.Runner{
			Stdin: null, Stdout: null, Stderr: null, Semantics: &sem, Diagnostics: &diag,
			Dir: dir, Name: "ksh", Route: interp.RouteCommandString,

			Dialect: presetDialect(),
			ReplaceProcess: func(string, string, []string, []string, []*os.File) error {
				return errors.New("recorded rather than replaced")
			},
		}).SeedVars(map[string]string{"PATH": "/usr/bin:/bin"})
		ksh.Apply(r)
		r.LastPart = true
		_, _ = r.Run(context.Background(), f)
		_ = null.Close()
		if got := r.TailReached(); got != tc.reached {
			t.Errorf("%s: TailReached = %v, want %v", tc.src, got, tc.reached)
		}
	}
}

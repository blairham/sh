// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/blairham/sh/interp"
)

// TestAPathThroughAFileIsNotFoundWhereTheDialectSaysSo is #6092: a trailing
// slash after a file reaches the kernel since #6089, which answers ENOTDIR,
// and the dialects part on what that is. With neither switch it is the
// kernel's reason at 126; Diagnostics.NotADirectoryExecStatus moves only the
// status; Diagnostics.NotADirectoryIsNotFound makes it a path that is not
// there, for the command and for a redirection's reason.
func TestAPathThroughAFileIsNotFoundWhereTheDialectSaysSo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "exe"), []byte("#!/bin/sh\necho ran\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	src := "./exe/; echo \"st=$?\"; read x < f/\n"
	for _, c := range []struct {
		name string
		diag Diagnostics
		want string
	}{
		{"the kernel's", Diagnostics{}, "sh: ./exe/: Not a directory\nst=126\nsh: cannot open f/: Not a directory\n"},
		{"status only", Diagnostics{NotADirectoryExecStatus: 127}, "sh: ./exe/: Not a directory\nst=127\nsh: cannot open f/: Not a directory\n"},
		{"not there", Diagnostics{NotADirectoryIsNotFound: true, FileNotFound: "No such file"}, "sh: ./exe/: not found\nst=127\nsh: cannot open f/: No such file\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := run(t, src, func(r *Runner) {
				d := c.diag
				r.Diagnostics, r.Dir = &d, dir
			})
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

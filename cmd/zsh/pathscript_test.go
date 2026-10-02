// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// TestPathscriptLooksTheScriptUpOnPath: `-o pathscript` on the invocation
// finds a slash-less script operand along PATH, which `zsh -f name` does not.
// Measured 2026-10-02 on zsh 5.9.2 (#5138), the A01grammar chunk: `-o
// pathscript`, `--pathscript`, `-o PATH_SCRIPT` and `+o nopathscript` run it,
// and no option, `-o nopathscript` and `-o pathscript +o pathscript` report
// `can't open input file`.
func TestPathscriptLooksTheScriptUpOnPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "myscript"), []byte("echo Found the script.\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	for _, c := range []struct {
		opts  []string
		found bool
	}{
		{[]string{"-o", "pathscript"}, true},
		{[]string{"--pathscript"}, true},
		{[]string{"-o", "PATH_SCRIPT"}, true},
		{[]string{"+o", "nopathscript"}, true},
		{nil, false},
		{[]string{"-o", "nopathscript"}, false},
		{[]string{"-o", "pathscript", "+o", "pathscript"}, false},
	} {
		var out, errs strings.Builder
		sh := scratchShell(t)
		sh.Stdout, sh.Stderr = &out, &errs
		sh.Env = []string{"PATH=" + dir, "HOME=" + t.TempDir()}
		argv := append(append([]string{"zsh", "-f"}, c.opts...), "myscript")
		code := driver.MainArgs(sh, argv)
		if c.found && (code != 0 || out.String() != "Found the script.\n") {
			t.Errorf("%v: got %q status %d stderr %q, want the script found", c.opts, out.String(), code, errs.String())
		}
		if !c.found && (code != 127 || !strings.Contains(errs.String(), "can't open input file: myscript")) {
			t.Errorf("%v: got %q status %d stderr %q, want it not looked for", c.opts, out.String(), code, errs.String())
		}
	}
}

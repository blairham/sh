// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// What this shell does *not* do with a `#!` line, which is the half a fix for
// the shell that does would quietly take with it — #4454, measured 2026-09-25
// against bash 5.3.20 and again against 3.2.57.

func writeShebangFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// A slashless `#!` word is not searched for here. `#!cat` is `cat: bad
// interpreter` at **126** in all three bash columns, where zsh finds `cat` on
// PATH and runs it on the file at 0.
func TestASlashlessInterpreterIsNotSearchedFor(t *testing.T) {
	dir, bin := t.TempDir(), t.TempDir()
	writeShebangFile(t, bin, "myint", "#!/bin/sh\necho INTERP ran\n")
	script := writeShebangFile(t, dir, "uses.scr", "#!myint\necho body\n")
	out, st, err := preset.Combined(t, dialecttest.Base{
		Dir:  dir,
		Vars: map[string]string{"PATH": dir + string(os.PathListSeparator) + bin},
	}, `uses.scr; echo "st=$?"`)
	if err != nil {
		t.Fatal(err)
	}
	want := "bash: " + script + ": myint: bad interpreter: No such file or directory\nst=126\n"
	if out != want || st != 0 {
		t.Errorf("a slashless interpreter:\n got %q at %d\nwant %q", out, st, want)
	}
	if strings.Contains(out, "INTERP ran") || strings.Contains(out, "body") {
		t.Errorf("nothing may have run: got %q", out)
	}
}

// And a `#!` naming nothing at all is read as a shell script here, which is
// what every column but zsh does with it.
func TestAnEmptyInterpreterLineIsRunAsAScript(t *testing.T) {
	dir := t.TempDir()
	writeShebangFile(t, dir, "e.scr", "#!\necho ran-anyway\n")
	out, st := runBash(t, dir, `./e.scr; echo "st=$?"`)
	want := "ran-anyway\nst=0\n"
	if out != want || st != 0 {
		t.Errorf("an empty `#!`:\n got %q at %d\nwant %q", out, st, want)
	}
}

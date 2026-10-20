// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// This shell reads a `#!` line itself rather than leaving it to the kernel,
// and the three rows below are what that is worth — #4454, measured
// 2026-09-25 against zsh 5.9.2 with `-f`.
//
// Every one of them was Go's `fork/exec <abs path>: no such file or
// directory` at 126 before, which is the only diagnostic of this shell's that
// read as a Go error: a sentence no shell writes, carrying a path nobody
// typed, about a file that is plainly there.

// runShebang runs src with the script in dir and the interpreter in a second
// directory that is on PATH and is not where the command runs.
//
// The separation is the fixture and not tidiness: a relative `#!` word is
// resolved by the *kernel* against the child's working directory, so an
// interpreter sitting beside its script is found before this shell is asked
// anything, and a row arranged that way would report a PATH search that never
// happened.
func runShebang(t *testing.T, dir, bin, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Dir:  dir,
		Vars: map[string]string{"PATH": dir + string(os.PathListSeparator) + bin},
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

func writeShebangFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// A `#!` naming a word with no slash in it is a PATH search here, and the
// file is then run by what the search found. `#!cat` prints the file.
//
// The interpreter writes its own argv, which is the discriminating half: a
// shell that had merely stopped complaining would print nothing, and one that
// read the file itself would print `body`.
func TestASlashlessInterpreterIsFoundOnPath(t *testing.T) {
	dir, bin := t.TempDir(), t.TempDir()
	writeShebangFile(t, bin, "myint", "#!/bin/sh\nprintf '[%s]' \"$0\" \"$@\"; echo\n")
	script := writeShebangFile(t, dir, "uses.scr", "#!myint\necho body\n")
	out, st := runShebang(t, dir, bin, `uses.scr a1; echo "st=$?"`)
	want := "[" + filepath.Join(bin, "myint") + "][" + script + "][a1]\nst=0\n"
	if out != want || st != 0 {
		t.Errorf("a slashless interpreter:\n got %q at %d\nwant %q at 0", out, st, want)
	}
}

// And when the search finds nothing, the file is named, the word its line
// held is named, and the status is **127** — the answer a script testing
// `$? -eq 127` for "not found" is reading, where bash says 126 for the same
// shape. The same sentence for a `#!` written as a path that is not there.
func TestAMissingInterpreterIsNamedAndIs127(t *testing.T) {
	for _, tc := range []struct {
		name string
		line string
		word string
	}{
		{"a path", "#!/nonexistent/interp\n", "/nonexistent/interp"},
		{"a bare word", "#!nosuchinterp_zz\n", "nosuchinterp_zz"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, bin := t.TempDir(), t.TempDir()
			script := writeShebangFile(t, dir, "bad.scr", tc.line+"echo SHOULD-NOT-RUN\n")
			out, st := runShebang(t, dir, bin, `bad.scr; echo "st=$?"`)
			want := "zsh:1: " + script + ": bad interpreter: " + tc.word +
				": no such file or directory\nst=127\n"
			if out != want || st != 0 {
				t.Errorf("a missing interpreter:\n got %q at %d\nwant %q", out, st, want)
			}
			if strings.Contains(out, "fork/exec") {
				t.Errorf("a Go error must not reach a shell diagnostic: got %q", out)
			}
		})
	}
}

// The other direction, and the one where this shell is the *stricter* one: a
// `#!` naming nothing at all is refused rather than read as a shell script.
// Every other column in the panel runs it and reports 0.
func TestAnEmptyInterpreterLineIsRefused(t *testing.T) {
	for _, line := range []string{"#!\n", "#!  \t \n"} {
		dir, bin := t.TempDir(), t.TempDir()
		writeShebangFile(t, dir, "e.scr", line+"echo ran-anyway\n")
		out, st := runShebang(t, dir, bin, `e.scr; echo "st=$?"`)
		want := "zsh:1: exec format error: e.scr\nst=126\n"
		if out != want || st != 0 {
			t.Errorf("an empty `#!` %q:\n got %q at %d\nwant %q", line, out, st, want)
		}
	}
}

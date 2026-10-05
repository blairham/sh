// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// **Tab marks a symlinked directory only once the word names it whole**
// (#5872), which is readline's default, `mark-symlinked-directories` off.
//
// Measured 2026-10-04 against `/opt/homebrew/bin/bash` — GNU bash 5.3.20 —
// through a pseudo-terminal, `--norc --noprofile -i`, with `realdir/` and
// `linkdir -> realdir`:
//
//	link⇥       linkdir      no slash, and nothing after it
//	link⇥⇥      linkdir/
//	real⇥       realdir/
//
// This shell put the slash on at once. The line is finished with an `x` typed
// straight after the Tabs and printed as `[%s]` words, so the three possible
// answers read differently: `[…/linkdirx]` for nothing after the name,
// `[…/linkdir][x]` for a space, and `[…/linkdir/x]` for a slash.
func TestTabMarksASymlinkedDirectoryOnceItIsWhole(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "realdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("realdir", filepath.Join(dir, "linkdir")); err != nil {
		t.Fatal(err)
	}
	control, screen := interruptSession(t, "")
	line := regexp.MustCompile(`out(\[[^\r\n]*\])\r?\n`)
	for _, tc := range []struct{ name, typed, want string }{
		{"one Tab on a link", "link\t", "[" + dir + "/linkdirx]"},
		{"two Tabs on a link", "link\t\t", "[" + dir + "/linkdir/x]"},
		{"one Tab on a directory", "real\t", "[" + dir + "/realdir/x]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := interruptAnswer(t, control, screen, "printf 'out'; printf '[%s]' "+dir+"/"+tc.typed+"x; echo", line)
			if got != tc.want {
				t.Errorf("printed %q, want %q", got, tc.want)
			}
		})
	}
}

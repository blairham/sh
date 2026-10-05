// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `${x?word}` in a file `.` reads at an interactive zsh's prompt ends the line
// the `.` was on, as it would end a script, and leaves 126 — the status a
// file `.` gave up over any other error reports (#6057). See
// interp.Diagnostics.SourcedFatalStatus.
//
// Measured 2026-10-05 on zsh 5.9.2 through a pseudo-terminal: `. ./f` then
// `echo X $?` writes `X 126`, `g(){ . ./f; echo in $?; }; g` writes no `in`
// and leaves 126 too, and `zsh -i -c '. ./f'` exits 126. This shell left 1.
func TestAParamErrorInDotTextAtAPromptLeaves126(t *testing.T) {
	for _, c := range []struct {
		typed, want string
		argv        []string
	}{
		{". ./f\necho X $?\n", "X 126\n", []string{"zsh", "-f", "-i"}},
		{"g(){ . ./f; echo in $?; }; g\necho X $?\n", "X 126\n", []string{"zsh", "-f", "-i"}},
		{"", "", []string{"zsh", "-f", "-i", "-c", ". ./f; echo in $?"}},
	} {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Chdir(home)
		if err := os.WriteFile(filepath.Join(home, "f"), []byte("(exit 3)\necho ${unset?boom}\necho after\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		out, errs, code := prompt(t, c.typed, c.argv...)
		if !strings.Contains(out, c.want) || strings.Contains(out, "in ") || strings.Contains(out, "after") {
			t.Errorf("%q %v: stdout %q, want %q and nothing after the failure (stderr %q)", c.typed, c.argv, out, c.want, errs)
		}
		if c.typed == "" && code != 126 {
			t.Errorf("%v: status %d, want 126 (stderr %q)", c.argv, code, errs)
		}
	}
}

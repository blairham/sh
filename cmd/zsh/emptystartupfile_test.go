// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// A startup file with no command in it leaves 0, where the other columns keep
// the status the file before it left (#5883).
//
// Measured 2026-10-05 on zsh 5.9.2 (/opt/homebrew/bin/zsh) under `env -u
// FPATH` with a scratch `$ZDOTDIR`, a `.zshenv` of `false` in front of each:
// an empty, blank or comment-only `.zshrc` answers `zsh -i -c 'echo st=$?'`
// with `st=0`, and so does an empty `.zlogin` under `-l -i -c`. The status is
// not cleared when the file starts — `echo inrc=$?` as the `.zshrc` prints
// `inrc=1` — and a `.zshrc` that is not there clears nothing. This shell
// left 1 in every row but the last two.
func TestAZshStartupFileThatRunsNothingLeavesZero(t *testing.T) {
	for _, c := range []struct {
		name  string
		files map[string]string
		argv  []string
		out   string
	}{
		{"an empty .zshrc", map[string]string{".zshenv": "false\n", ".zshrc": ""}, []string{"zsh", "-i", "-c", "echo st=$?"}, "st=0\n"},
		{"a .zshrc of blank lines", map[string]string{".zshenv": "false\n", ".zshrc": "\n\n  \n"}, []string{"zsh", "-i", "-c", "echo st=$?"}, "st=0\n"},
		{"a .zshrc of a comment", map[string]string{".zshenv": "false\n", ".zshrc": "# nothing\n"}, []string{"zsh", "-i", "-c", "echo st=$?"}, "st=0\n"},
		{"an empty .zlogin", map[string]string{".zshenv": "false\n", ".zlogin": ""}, []string{"zsh", "-l", "-i", "-c", "echo st=$?"}, "st=0\n"},
		// The controls: the status reaches the file, and a file that is not
		// there is not a file with nothing in it.
		{"the status reaches the file", map[string]string{".zshenv": "false\n", ".zshrc": "echo inrc=$?\n"}, []string{"zsh", "-i", "-c", "echo st=$?"}, "inrc=1\nst=0\n"},
		{"no .zshrc at all", map[string]string{".zshenv": "false\n"}, []string{"zsh", "-i", "-c", "echo st=$?"}, "st=1\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := scratchHome(t)
			for name, body := range c.files {
				writeHomeFile(t, home, name, body)
			}
			var o, e bytes.Buffer
			sh := scratchShell(t)
			sh.Stdout, sh.Stderr = &o, &e
			driver.MainArgs(sh, c.argv)
			if o.String() != c.out {
				t.Errorf("stdout %q, want %q (stderr %q)", o.String(), c.out, e.String())
			}
		})
	}
}

// And at the first prompt, which is where a person reads it.
func TestAnEmptyZshrcLeavesZeroAtThePrompt(t *testing.T) {
	home := scratchHome(t)
	writeHomeFile(t, home, ".zshenv", "false\n")
	writeHomeFile(t, home, ".zshrc", "")
	out, errs, _ := prompt(t, "echo st=$?\n", "zsh", "-i")
	if !strings.Contains(out, "st=0") || strings.Contains(out, "st=1") {
		t.Errorf("said %q, want st=0 (stderr %q)", out, errs)
	}
}

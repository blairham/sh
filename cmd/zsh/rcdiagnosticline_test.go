// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// A run-time diagnostic from the top level of `.zshrc` is located by the
// file's path and line at a prompt, exactly as under `-c` (#5870).
//
// Measured 2026-10-05 on zsh 5.9.2 (/opt/homebrew/bin/zsh) under `env -u
// FPATH`, scratch `HOME`=`ZDOTDIR`, `zsh -i` with the program on a pipe; each
// line is that shell's with the home directory standing for `@`. This shell
// wrote the prompt's shape — `@/.zshrc: command not found: nosuch2` with no
// line, and `source: no such file or directory: …` with no location at all.
func TestAZshrcDiagnosticAtAPromptHasItsLine(t *testing.T) {
	for _, c := range []struct{ line, want string }{
		{"nosuch2", "@/.zshrc:2: command not found: nosuch2\n"},
		{"source /nonexistent/x.zsh", "@/.zshrc:source:2: no such file or directory: /nonexistent/x.zsh\n"},
		{". /nonexistent/z", "@/.zshrc:.:2: no such file or directory: /nonexistent/z\n"},
		{"cd /nonexistent", "@/.zshrc:cd:2: no such file or directory: /nonexistent\n"},
		{"print ${unset?boom}", "@/.zshrc:2: unset: boom\n"},
		// The control: a function the file defines names itself, on both.
		{"f(){ source /nonexistent/y }; f", "f:source: no such file or directory: /nonexistent/y\n"},
	} {
		// The prompt is drawn on the same stream, and is taken out below.
		home := scratchHome(t)
		writeHomeFile(t, home, ".zshrc", "PS1='> '\n"+c.line+"\n")
		_, errs, _ := prompt(t, "exit\n", "zsh", "-i")
		errs = strings.ReplaceAll(errs, "> ", "")
		if want := strings.ReplaceAll(c.want, "@", home); errs != want {
			t.Errorf("%s: stderr %q, want %q", c.line, errs, want)
		}
	}
}

// And a line typed at the prompt keeps the prompt's shape, which is the half
// of the rule the file must not reach: `zsh: command not found: nosuch3`.
func TestATypedLineKeepsThePromptsShapeAfterAZshrc(t *testing.T) {
	home := scratchHome(t)
	writeHomeFile(t, home, ".zshrc", "PS1='> '\nnosuch2\n")
	_, errs, _ := prompt(t, "nosuch3\n", "zsh", "-i")
	// The prompt is drawn on the same stream; it is not what is graded.
	errs = strings.ReplaceAll(errs, "> ", "")
	if want := home + "/.zshrc:2: command not found: nosuch2\nzsh: command not found: nosuch3\n"; errs != want {
		t.Errorf("stderr %q, want %q", errs, want)
	}
}

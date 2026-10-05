// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// `.zlogout` is read from the middle of whatever stopped the shell, so
// `$zsh_eval_context` there holds what the shell was inside — cut at the first
// function for an `exit`, whole for a hangup — and the file alone where the
// program ran out (#6005). See interp.Runner.captureLeavingContexts.
//
// Measured 2026-10-05 on zsh 5.9.2 (/opt/homebrew/bin/zsh) under `env -u
// FPATH`, `zsh -l -i -c` with standard input on the null device; this shell
// read the file over an empty stack every time.
func TestAZlogoutIsReadFromWhereTheShellStopped(t *testing.T) {
	for _, c := range []struct{ cmd, want string }{
		{"exit", "cmdarg file"},
		{". ./ex.zsh", "cmdarg file file"},
		{". ./ex2.zsh", "cmdarg file file file"},
		{"eval 'exit 4'", "cmdarg eval file"},
		{`trap "exit 5" USR1; kill -USR1 $$`, "cmdarg trap file"},
		{"f(){ . ./ex.zsh }; f", "cmdarg file"},
		{"kill -HUP $$", "cmdarg file"},
		{"f(){ kill -HUP $$ }; f", "cmdarg shfunc file"},
		{". ./hup.zsh", "cmdarg file file"},
		{":", "file"},
		{"print ${u?x}", "file"},
	} {
		t.Run(c.cmd, func(t *testing.T) {
			home := scratchHome(t)
			writeHomeFile(t, home, ".zlogout", "print -r -- ctx=$zsh_eval_context\n")
			writeHomeFile(t, home, "ex.zsh", "exit 2\n")
			writeHomeFile(t, home, "ex2.zsh", ". ./ex.zsh\n")
			writeHomeFile(t, home, "hup.zsh", "kill -HUP $$\n")
			// Quoted: the scratch directory is named after the subtest, which
			// holds the command's own parentheses and braces.
			out, errs, _ := prompt(t, "", "zsh", "-l", "-i", "-c", "cd '"+home+"'; "+c.cmd)
			if !strings.Contains(out, "ctx="+c.want+"\n") {
				t.Errorf("stdout %q, want ctx=%s (stderr %q)", out, c.want, errs)
			}
		})
	}
}

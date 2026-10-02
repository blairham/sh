// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// TestAnEmptyArgv0IsTheShellsName: started under an empty argv[0], as `exec -a "" zsh -fc …` starts it, the
// shell's $0 is empty, where it used to fall back to the configured name.
// Measured 2026-10-02: zsh 5.9.2 writes `xx` for `print -r -- "x${0}x"`, as
// bash 5.3.20, ksh93u+ and dash do for their own spelling of it, and still
// names itself `zsh` in a diagnostic (#5138).
//
// The control is a non-empty argv[0], which must still win over the
// configured name.
func TestAnEmptyArgv0IsTheShellsName(t *testing.T) {
	for _, c := range []struct{ argv0, out string }{
		{"", "xx\n"},
		{"foo*", "xfoo*x\n"},
	} {
		var out, errs strings.Builder
		sh := scratchShell(t)
		sh.Stdout, sh.Stderr = &out, &errs
		driver.MainArgs(sh, []string{c.argv0, "-fc", `print -r -- "x${0}x"; nosuchcommand`})
		if out.String() != c.out {
			t.Errorf("argv[0] %q: stdout %q, want %q", c.argv0, out.String(), c.out)
		}
		if want := "zsh:1: command not found: nosuchcommand\n"; errs.String() != want {
			t.Errorf("argv[0] %q: stderr %q, want %q", c.argv0, errs.String(), want)
		}
	}
}

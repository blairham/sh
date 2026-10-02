// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// **`$ZSH_EXECUTION_STRING` is a `-c` program's own text**, an ordinary
// scalar set before the program runs and absent on every other route
// (#5157). Measured 2026-10-01 on zsh 5.9.2 under `-f`. Through the front end,
// because the text is the invocation's and only the driver holds it.
func TestTheExecutionStringIsTheCommandStringsText(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"the text", `print -r -- "[$ZSH_EXECUTION_STRING]"; print ${(t)ZSH_EXECUTION_STRING}`,
			`[print -r -- "[$ZSH_EXECUTION_STRING]"; print ${(t)ZSH_EXECUTION_STRING}]` + "\nscalar\n",
		},
		{
			"an ordinary scalar", `ZSH_EXECUTION_STRING=x; print $ZSH_EXECUTION_STRING; unset ZSH_EXECUTION_STRING; print ${+ZSH_EXECUTION_STRING}`,
			"x\n0\n",
		},
		{"no ZSH_SCRIPT beside it", `typeset | /usr/bin/grep -c '^ZSH_SCRIPT=' || true`, "0\n"},
		{"listed", `typeset -m 'ZSH_(EXECUTION_STRING|SCRIPT)'`, `ZSH_EXECUTION_STRING='typeset -m '\''ZSH_(EXECUTION_STRING|SCRIPT)'\'` + "\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, code := runZsh(t, "-f", "-c", c.src)
			if out != c.want || errs != "" || code != 0 {
				t.Errorf("%s\ngot  %q %q %d\nwant %q", c.src, out, errs, code, c.want)
			}
		})
	}
	// The control: a script file has none, and the listing names the
	// script's own parameter instead.
	script := filepath.Join(t.TempDir(), "s.zsh")
	if err := os.WriteFile(script, []byte("print ${+ZSH_EXECUTION_STRING}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, errs, code := runZsh(t, "-f", script); out != "0\n" || errs != "" || code != 0 {
		t.Errorf("a script file: got %q %q %d, want 0", out, errs, code)
	}
}

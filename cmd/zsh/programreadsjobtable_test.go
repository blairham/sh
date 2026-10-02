// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// TestAProgramRunInTheForegroundReadsTheJobTable: a `+` a command left on a
// number nobody holds is taken off by a program the shell runs and waits
// for, as `jobs` takes it off — and not by a builtin. Measured 2026-10-02 on
// zsh 5.9.2 (`/opt/homebrew/bin/zsh -fc`). See
// interp.Runner.forgetANumberNobodyHolds.
func TestAProgramRunInTheForegroundReadsTheJobTable(t *testing.T) {
	const f = `f() { /bin/sleep 0 & wait }; g() { wait %%; wait %- }; f; `
	for _, c := range []struct{ mid, want string }{
		{"/usr/bin/true; ", "g:wait: no current job\ng:wait: no previous job\n"},
		{"true; ", "g:wait: %%: no such job\ng:wait: no previous job\n"},
	} {
		_, errs, _ := runZsh(t, "-fc", f+c.mid+"g")
		if errs != c.want {
			t.Errorf("%s: stderr %q, want %q", c.mid, errs, c.want)
		}
	}
}

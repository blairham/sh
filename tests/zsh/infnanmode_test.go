// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestOnlyTheShModeReadsInfAndNaNAsParameters is #5157's E03posix row `All
// identifiers are variable references in POSIX arithmetic`. Under sh, `inf`
// and `nan` are ordinary names in arithmetic. Under the other three modes they
// are the floating constants. The emulation table in emulate.go carries the
// rule.
//
// Every row measured 2026-10-02 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`).
func TestOnlyTheShModeReadsInfAndNaNAsParameters(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`emulate zsh; inf=42; nan=7; echo $((inf)) $((nan)) $((Inf))`, "Inf NaN Inf\n"},
		{`emulate sh; inf=42; nan=7; echo $((inf)) $((nan)) $((Inf))`, "42 7 0\n"},
		{`emulate ksh; inf=42; nan=7; echo $((inf)) $((nan)) $((Inf))`, "Inf NaN Inf\n"},
		{`emulate csh; inf=42; nan=7; echo $((inf)) $((nan)) $((Inf))`, "Inf NaN Inf\n"},
		// Back again, and through each of the other ways into the mode.
		{`emulate sh; emulate zsh; inf=42; echo $((inf))`, "Inf\n"},
		{`inf=42; emulate -l sh >/dev/null; echo $((inf))`, "42\n"},
		{`inf=42; emulate sh -c 'f() { echo $((inf)); }'; f; echo $((inf))`, "42\nInf\n"},
		{`inf=42; g() { emulate -L sh; echo $((inf)); }; g; echo $((inf))`, "42\nInf\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), c.src+"\n")
		if out != c.want {
			t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
		}
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// A fatal error in the text of `.` or `eval` in an interactive bash costs the
// line of that text it was on, and the text reads on (#6057). See
// interp.Semantics.BorrowedTextErrorWhenInteractiveCostsTheLine.
//
// Measured 2026-10-05 on bash 5.3.20 through a pseudo-terminal and on a pipe
// alike, f holding `(exit 3)`, the failing line with `; echo same` behind it,
// and `echo after $?`. This shell gave up the whole text and wrote `X 1`.
func TestAnErrorInDotTextAtAPromptCostsTheLine(t *testing.T) {
	for _, line := range []string{"echo ${unset?boom}", "set -u; echo $nope", "g(){ echo ${unset?boom}; }; g"} {
		for _, c := range []struct {
			typed, want string
			argv        []string
		}{
			{". ./f\necho X $?\n", "after 1\nX 0\n", []string{"bash", "--norc", "-i"}},
			{"eval \"$(cat f)\"\necho X $?\n", "after 1\nX 0\n", []string{"bash", "--norc", "-i"}},
			{"", "after 1\nin 0\n", []string{"bash", "--norc", "-i", "-c", ". ./f; echo in $?"}},
			// A subshell is not interactive, and gives the text up.
			{"", "X 1\n", []string{"bash", "--norc", "-i", "-c", "( . ./f; echo in $? ); echo X $?"}},
		} {
			home := scratchHome(t)
			t.Chdir(home)
			writeHomeFile(t, home, "f", "(exit 3)\n"+line+"; echo same\necho after $?\n")
			out, errs, _ := prompt(t, c.typed, c.argv...)
			if !strings.Contains(out, c.want) || strings.Contains(out, "same") {
				t.Errorf("%q, %q %v: stdout %q, want %q and no `same` (stderr %q)", line, c.typed, c.argv, out, c.want, errs)
			}
		}
	}
}

// And the status a failed expansion leaves under `-i -c` is the 1 an
// interactive shell's is, not the 127 the same string exits with when the
// shell is not interactive: measured 2026-10-05 on bash 5.3.20,
// `bash -i -c 'echo ${unset?boom}'` exits 1 (#6057).
func TestAFailedExpansionUnderInteractiveCommandStringIsOne(t *testing.T) {
	scratchHome(t)
	_, errs, code := prompt(t, "", "bash", "--norc", "-i", "-c", "echo ${unset?boom}")
	if code != 1 {
		t.Errorf("status %d, want 1 (stderr %q)", code, errs)
	}
}

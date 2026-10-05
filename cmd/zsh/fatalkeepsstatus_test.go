// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// A failed expansion in a command's words leaves a failing `$?` standing and
// sets 1 over a 0, outside a loop; `${x?word}` sets 1 unless the shell is
// interactive (#6067). See
// interp.Semantics.FailedExpansionInACommandKeepsAFailingStatus.
//
// Measured 2026-10-05 on zsh 5.9.2 (`zsh -f`); every expected status is that
// shell's. This shell exited 1 on every row.
func TestAFailedExpansionKeepsAFailingStatus(t *testing.T) {
	for _, c := range []struct {
		src  string
		want int
	}{
		{"(exit 4); print $((1/0))", 4},
		{"(exit 4); print a ${x!!}", 4},
		{"(exit 4); x=1 builtin print ~nosuch", 4},
		{"setopt nounset; (exit 4); print $nope", 4},
		{"f(){ (exit 6); print $((1/0)); }; f", 6},
		{"(exit 4); echo $(exit 5) $((1/0))", 5},
		{"(exit 4); { echo $((1/0)); } always { :; }", 4},
		// The controls: a 0 becomes 1, and so do these.
		{"true; print $((1/0))", 1},
		{"(exit 4); x=$((1/0))", 1},
		{"(exit 4); print ${x?boom}", 1},
		{"f(){ (exit 6); print $((1/0)); }; for i in 1; do f; done", 1},
	} {
		t.Setenv("HOME", t.TempDir())
		_, errs, code := prompt(t, "", "zsh", "-f", "-c", c.src)
		if code != c.want {
			t.Errorf("%q: status %d, want %d (stderr %q)", c.src, code, c.want, errs)
		}
	}
}

// And at a prompt, `${x?word}` keeps it too: `(exit 3); echo ${unset?boom}`
// then `echo X $?` is `X 3`, and so is the same in `eval`.
func TestAParamErrorAtAPromptKeepsAFailingStatus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	out, errs, _ := prompt(t, "(exit 3); echo ${unset?boom}\necho X $?\ntrue; echo ${unset?boom}\necho X $?\n"+
		"(exit 3); eval \"echo \\${unset?boom}\"\necho X $?\n", "zsh", "-f", "-i")
	if !strings.Contains(out, "X 3\nX 1\nX 3\n") {
		t.Errorf("stdout %q, want X 3, X 1, X 3 (stderr %q)", out, errs)
	}
}

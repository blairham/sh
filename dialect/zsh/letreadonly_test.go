// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// This shell ends a script for a builtin's refused write to its own output
// parameter — `read` and `printf -v` both do — and it does *not* for `let`.
// Ours took the builtin's answer there, so the script was over.
//
// Measured 2026-09-18 against zsh 5.9.2 over a script file, `env -i` with
// LC_ALL=C (#3470).
func TestALetRefusedByAFreezeIsNotFatalHere(t *testing.T) {
	out, st := answersRun(t, "readonly x=1\nlet x=2\necho after $?\n")
	if !strings.Contains(out, "read-only variable: x") {
		t.Errorf("got %q, want the refusal reported", out)
	}
	if !strings.Contains(out, "after 1") {
		t.Errorf("got %q at %d, want the next line to run and report 1", out, st)
	}
}

// And the two it *is* fatal for, which is what says the answer is `let`'s and
// not this shell's in general. `(( ))` is the third reading again: reported,
// not fatal, and at the arithmetic command's own status rather than a
// builtin's.
func TestTheNeighboursOfThatRefusalAreUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		tail      bool
	}{
		{"printf -v ends the script", "readonly x=1\nprintf -v x hi\necho after $?\n", false},
		{"read ends it too", "readonly x=1\necho v | read x\necho after $?\n", false},
		{"and the arithmetic command does not", "readonly x=1\n(( x = 2 ))\necho after $?\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if got := strings.Contains(out, "after"); got != tc.tail {
				t.Errorf("got %q, want the next line to run: %v", out, tc.tail)
			}
			if tc.tail && !strings.Contains(out, "after 2") {
				t.Errorf("got %q, want the arithmetic command's own status", out)
			}
		})
	}
}

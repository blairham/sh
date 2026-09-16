// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// `-o interactive` is `-i` spelled in the option namespace, and the shell has
// to *act* interactive for it — #3195.
//
// The option already reported itself: `$-` gained `i` and `Z`, `[[ -o
// interactive ]]` was true and `set -o` listed the row on, all byte for byte
// with real zsh. What it did not do was draw a prompt, because the route
// decision was taken from the letter loop alone, before the invocation's `-o`
// options reached the runner at all. So a test that read the option would have
// passed throughout the life of the bug, and every case here asserts on the
// *effect* instead: a prompt drawn, and the run-commands file read — which an
// interactive zsh reads and a script does not.
//
// The prompt is two rows on purpose. A one-line `PS1` is drawn by parts of
// this front end that a two-row one is not, and a probe that cannot tell them
// apart has measured the easy half. **Both** rows are asserted, which they
// were not when this was written: the plain prompt loop drew the last row and
// dropped the ones above it, so this test asserted `ROW2> ` alone and said so.
// That was #3222, and with it fixed the upper row is the half that says the
// prompt reaching a program on the other end of a pipe is the whole prompt.
//
// Measured 2026-09-16 against zsh 5.9.2 with the program on a pipe, so that
// nothing but the invocation could make the shell interactive. Each row below
// is that shell's answer.
func TestTheInteractiveOptionNameDrawsAPrompt(t *testing.T) {
	for _, tc := range []struct {
		name    string
		argv    []string
		prompts bool
	}{
		{"the option alone", []string{"zsh", "-o", "interactive"}, true},
		{"the letter, for comparison", []string{"zsh", "-i"}, true},
		{"nothing written at all", []string{"zsh"}, false},
		// The other direction, and the namespace's own negative spelling of
		// it. Both are the invocation saying no rather than saying nothing.
		{"the option under a plus", []string{"zsh", "+o", "interactive"}, false},
		{"the negated name", []string{"zsh", "-o", "nointeractive"}, false},
		{"the negated name under a plus", []string{"zsh", "+o", "nointeractive"}, true},
		// The letter and the name are one last-wins sequence, which is the
		// half a set of single-spelling rows cannot reach: each of these
		// writes both, and only the order differs.
		{"the letter then the option", []string{"zsh", "-i", "+o", "interactive"}, false},
		{"the option then the letter", []string{"zsh", "+o", "interactive", "-i"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := scratchHome(t)
			writeHomeFile(t, home, ".zshrc", "PS1=$'ROW1\\nROW2> '\nprint -r -- RCWASREAD\n")
			out, errs, code := prompt(t, "echo TYPED\n", tc.argv...)
			both := out + errs
			if code != 0 {
				t.Fatalf("status %d, out %q, stderr %q", code, out, errs)
			}
			// The line was run either way, so a row that drew no prompt is a
			// shell that read the program rather than a shell that did
			// nothing.
			if !strings.Contains(both, "TYPED") {
				t.Fatalf("said %q, want the typed line to have run under either answer", both)
			}
			for _, mark := range []string{"RCWASREAD", "ROW1\nROW2> "} {
				if got := strings.Contains(both, mark); got != tc.prompts {
					t.Errorf("%q present = %v, want %v — said %q", mark, got, tc.prompts, both)
				}
			}
		})
	}
}

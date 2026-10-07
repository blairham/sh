// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// A backslash that is the last byte of a prompt's input is read the way the
// end of a command string reads one in dash, ksh93 and BusyBox ash, and as a
// continuation the end of the input finishes in bash and zsh. Measured
// 2026-10-07 on a pipe, `echo A \` with no newline: `A \` in the three, `A`
// in the two (#6337). See
// interp.Semantics.PromptBackslashTheInputEndsOnIsLiteral. The zero
// syntax.Dialect keeps the backslash at the end of the input, which is the
// reading the three have under `-c`.
func TestABackslashTheInputEndsOnAtAPrompt(t *testing.T) {
	for _, c := range []struct {
		name    string
		literal bool
		in      string
		out     string
		errs    string
	}{
		{"a word of its own", true, "echo A \\", "A \\\n", "P> P> "},
		{"a continuation the end finishes", false, "echo A \\", "A\n", "P> Q> P> "},
		{"a backslash before a newline continues either way", true, "echo A \\\n", "A\n", "P> Q> P> "},
	} {
		t.Run(c.name, func(t *testing.T) {
			var ran, said strings.Builder
			r := newTestRunner(map[string]string{"PS1": "P> ", "PS2": "Q> "})
			r.Stdout = &ran
			s := Shell{
				Runner: r, In: strings.NewReader(c.in), Out: &ran, Err: &said,
				BackslashTheInputEndsOnIsLiteral: c.literal,
			}
			if _, err := s.Run(t.Context()); err != nil {
				t.Fatal(err)
			}
			if ran.String() != c.out {
				t.Errorf("stdout %q, want %q", ran.String(), c.out)
			}
			if said.String() != c.errs {
				t.Errorf("stderr %q, want %q", said.String(), c.errs)
			}
		})
	}
}

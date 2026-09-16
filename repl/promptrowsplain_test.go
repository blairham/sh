// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"path/filepath"
	"strings"
	"testing"
)

// A multi-row prompt loses every row but the last wherever there is no line
// editor.
//
// drawnPrompt is split at its last newline because the *editor* needs the two
// halves at different times: the rows above are written once and the last row
// is rewritten at every keystroke, which is what stopped a two-row prompt
// leaving a ladder behind it (#2467). The loop that reads from something that
// is not a terminal redraws nothing at all, and it wrote only the half the
// editor redraws — so `PS1=$'A1\nA2\nA3> '` on a pipe drew `A3> `, and the
// rows carrying the directory, the branch and the status went missing, leaving
// the one character that looks identical in every shell (#3222).
//
// **Through a pseudo-terminal the same prompt is drawn whole**, measured the
// same day on the shipped binary with internal/cmd/tworowprobe: `UPPERROW\r\n`
// then `READY> ` on the wire, before and after. So this is the plain loop and
// not the prompt expansion — which is why the rows below drive the loop rather
// than drawPrompt, whose own split promptrows_test.go already pins.
//
// The panel, measured 2026-09-16 with that prompt and `echo T` on a pipe,
// `env -i` with a scratch HOME. Every column writes every row, at the first
// prompt and at the second:
//
//	bash 5.3.20       A1 / A2 / A3>
//	bash as `sh`      A1 / A2 / A3>
//	bash 3.2.57       A1 / A2 / A3>
//	zsh 5.9.2         A1 / A2 / A3>   (behind its own `%  \r \r` partial-line mark)
//	ksh93u+ 2012      A1 / A2 / A3>
//	dash 0.5.12       A1 / A2 / A3>
//	BusyBox ash 1.37  A1 / A2 / A3>
//	ours, before      A3>
func TestAPlainPromptIsDrawnWhole(t *testing.T) {
	for _, tc := range []struct{ name, ps1, ps2, typed, want string }{
		{
			"two rows", "A1\nA2> ", "", "echo T\n",
			"A1\nA2> A1\nA2> ",
		},
		{
			// Three rows lose two, which is what says the whole lead is
			// written and not merely the row above the last.
			"three rows", "A1\nA2\nA3> ", "", "echo T\n",
			"A1\nA2\nA3> A1\nA2\nA3> ",
		},
		{
			// The ordinary prompt, and the row that says this cost it
			// nothing.
			"one row", "$ ", "", "echo T\n",
			"$ $ ",
		},
		{
			// The continuation prompt is drawn by the same call and lost its
			// rows the same way: measured on zsh 5.9.2, `B1` is written
			// before each continued line.
			"a continuation prompt", "A1\nA2> ", "B1\nB2> ", "for i in 1\ndo\n:\ndone\n",
			"A1\nA2> B1\nB2> B1\nB2> B1\nB2> A1\nA2> ",
		},
		{
			// A prompt that is nothing but a newline puts the line at the
			// left margin, and the newline is still written.
			"a prompt ending in a newline", "A1\n", "", "echo T\n",
			"A1\nA1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := plainPrompts(t, tc.ps1, tc.ps2, tc.typed); got != tc.want {
				t.Errorf("drew %q, want %q", got, tc.want)
			}
		})
	}
}

// plainPrompts runs a session whose input is not a terminal — the loop this is
// about — and returns everything that reached the error stream, which for
// these rows is the prompts and nothing else.
func plainPrompts(t *testing.T, ps1, ps2, typed string) string {
	t.Helper()
	var out, errs strings.Builder
	r := newTestRunner(map[string]string{
		"PS1": ps1, "PS2": ps2,
		// A file of this test's own. The session appends what it read, and
		// reading the machine's history would make the numbering differ per
		// machine as well as reading somebody's file.
		"HISTFILE": filepath.Join(t.TempDir(), "history"),
	})
	r.Stdout = &out
	s := Shell{Runner: r, In: readerFile(t, typed), Out: &out, Err: &errs}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	return errs.String()
}

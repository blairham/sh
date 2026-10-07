// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"io"
	"strings"
	"testing"
)

// What a prompt reading a pipe takes and has not used is gone from the
// descriptor, so a `read` typed at it finds only what is after it. Measured
// 2026-10-07, `read x` / `DATA` / `echo "[$x]"` in one write to `-i`: bash,
// zsh and ksh93 print `[DATA]`, and dash and BusyBox ash print `[]` — unless
// `DATA` starts exactly where their block ends (#6328). See
// interp.Semantics.PromptReadSize.
func TestAReadAtAPromptFindsWhatThePromptLeft(t *testing.T) {
	const small = "read x\nDATA\necho \"[$x]\"\n"
	// `DATA` at byte at, after `read x` and blank lines; the blank lines are
	// what a line-at-a-time prompt hands the `read` instead.
	at := func(at int) string {
		return "read x\n" + strings.Repeat("\n", at-7) + "DATA\necho \"[$x]\"\n"
	}
	for _, c := range []struct {
		name string
		size int
		in   string
		want string
	}{
		{"a line at a time leaves the next line for the read", 0, small, "[DATA]\n"},
		{"a block takes it", 1024, small, "[]\n"},
		{"unless the block ends right before it", 1024, at(1024), "[DATA]\n"},
		{"and a byte later it has gone", 1024, at(1025), "[]\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var ran strings.Builder
			in := strings.NewReader(c.in)
			r := newTestRunner(map[string]string{"PS1": "", "PS2": ""})
			r.Stdout = &ran
			r.Stdin = in
			s := Shell{Runner: r, In: in, Out: &ran, Err: io.Discard, ReadSize: c.size}
			if _, err := s.Run(t.Context()); err != nil {
				t.Fatal(err)
			}
			if ran.String() != c.want {
				t.Errorf("stdout %q, want %q", ran.String(), c.want)
			}
		})
	}
}

// And the same through a descriptor that cannot be rewound, which is the case
// that has to read a byte at a time to take nothing past the line.
func TestAReadAtAPromptOnAPipeFindsTheNextLine(t *testing.T) {
	pr, pw := io.Pipe()
	go func() {
		_, _ = io.WriteString(pw, "read x\nDATA\necho \"[$x]\"\n")
		_ = pw.Close()
	}()
	var ran strings.Builder
	r := newTestRunner(map[string]string{"PS1": "", "PS2": ""})
	r.Stdout = &ran
	r.Stdin = pr
	s := Shell{Runner: r, In: pr, Out: &ran, Err: io.Discard}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if ran.String() != "[DATA]\n" {
		t.Errorf("stdout %q, want [DATA]", ran.String())
	}
}

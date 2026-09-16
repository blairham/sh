// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// `.` of `/dev/stdin` runs the command's own standard input — a here-document,
// a here-string or a pipe — and not whatever the process was started with.
// Unanimous: bash 5.3.20, zsh 5.9.2, ksh93u+ and dash (the rows it has) all
// run the text, measured 2026-09-16 from a script file with the process's
// standard input on /dev/null. Here it ran nothing, at status 0 (#3402).
func TestDotOfStdinRunsTheCommandsOwnInput(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"a here-document", ". /dev/stdin <<'E'\necho doc\nE\n", "doc\n"},
		{"a pipe", "echo 'echo piped' | . /dev/stdin\n", "piped\n"},
		{"by descriptor number", ". /dev/fd/0 <<'E'\necho fd0\nE\n", "fd0\n"},
		{"with operands", ". /dev/stdin one two <<'E'\necho \"$1 $2\"\nE\n", "one two\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, status := axisRun(t, c.src, func(s *Semantics) {
				s.DotPassesArguments = Yes
				s.LastPipelineElementInCurrentShell = No
			})
			if out != c.want || status != 0 {
				t.Errorf("wrote %q at %d, want %q at 0", out, status, c.want)
			}
		})
	}
}

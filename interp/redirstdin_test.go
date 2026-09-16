// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// A redirection from `/dev/stdin` or `/dev/fd/0` reads the command's own
// standard input — a here-string or a pipe into a function — and not what the
// process was started with. Unanimous: every row is the same in bash 5.3.20,
// zsh 5.9.2 and ksh93u+, measured 2026-09-16 from a script file with the
// process's standard input on /dev/null. Here the function read nothing
// (#3404).
func TestARedirectionFromStdinReadsTheCommandsOwnInput(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"a here-string into a function", `f() { read l < /dev/stdin; echo "[$l]"; }; f <<<x`, "[x]\n"},
		{"by descriptor number", `f() { read l < /dev/fd/0; echo "[$l]"; }; f <<<y`, "[y]\n"},
		{"an external command", "g() { cat < /dev/stdin; }; g <<<z", "z\n"},
		{"one stream read twice", `k() { read a; read b < /dev/stdin; echo "[$a|$b]"; }; printf '1\n2\n' | k`, "[1|2]\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, status := axisRun(t, c.src, func(s *Semantics) {
				s.LastPipelineElementInCurrentShell = No
			})
			if out != c.want || status != 0 {
				t.Errorf("wrote %q at %d, want %q at 0", out, status, c.want)
			}
		})
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// Under sh emulation the last element of a pipeline runs in a subshell, and
// under zsh's own mode, ksh and csh it runs in this shell — the
// Semantics.LastPipelineElementInCurrentShell axis, moved by the mode. This is
// `B07emulate`'s last chunk, "emulate sh uses subshell for last pipe entry".
//
// Measured on zsh 5.9.2, `-f`, 2026-09-30. On main before this change every sh
// row read the zsh answer.
func TestTheEmulationDecidesWhereTheLastPipelineElementRuns(t *testing.T) {
	rows := []struct{ name, src, want string }{
		{"sh: an assignment", "emulate sh -c 'x=1; echo | x=2; echo x=$x'", "x=1\n"},
		{"sh: read", "emulate sh -c 'y=0; echo 5 | read y; echo y=$y'", "y=0\n"},
		{"sh: a function", "emulate sh -c 'g(){ z=9; }; z=0; echo | g; echo z=$z'", "z=0\n"},
		{"sh: a brace group", "emulate sh -c 'w=0; echo | { w=3; }; echo w=$w'", "w=0\n"},
		{"sh: a bare emulate", "emulate sh; x=1; echo | x=2; echo x=$x", "x=1\n"},
		{"sh: a sticky function", "emulate sh -c 'h(){ x=1; echo | x=2; echo x=$x; }'; h", "x=1\n"},
		{"sh: emulate -L inside a function", "f(){ emulate -L sh; x=1; echo | x=2; echo x=$x; }; f", "x=1\n"},
		{
			"the -c run is left behind it", "emulate sh -c 'x=1; echo | x=2; echo in=$x'; x=1; echo | x=2; echo out=$x",
			"in=1\nout=2\n",
		},
		{"and so is an emulate -L at the return", "f(){ emulate -L sh; }; f; x=1; echo | x=2; echo x=$x", "x=2\n"},
		{"ksh runs it here", "emulate ksh -c 'x=1; echo | x=2; echo x=$x'", "x=2\n"},
		{"csh runs it here", "emulate csh -c 'x=1; echo | x=2; echo x=$x'", "x=2\n"},
		{"zsh runs it here again after sh", "emulate sh; emulate zsh; x=1; echo | x=2; echo x=$x", "x=2\n"},
		{"and in a zsh -c run from sh", "emulate sh; emulate zsh -c 'x=1; echo | x=2; echo x=$x'", "x=2\n"},
		{"control: zsh runs it here", "x=1; echo | x=2; echo x=$x", "x=2\n"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), row.src)
			if st != 0 || out != row.want {
				t.Errorf("out %q status %d, want %q", out, st, row.want)
			}
		})
	}
}

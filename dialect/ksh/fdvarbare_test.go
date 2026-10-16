// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// TestABareFdVariableListIsTheShells — the No side of
// interp.Semantics.FdVariableMakesABareListASubshell: a command of
// redirections alone makes its `{name}` in the shell, so the name holds the
// number afterwards and a target's expansion stays (#6630). ksh93u+
// 2012-08-01 `-c`, measured 2026-10-15.
func TestABareFdVariableListIsTheShells(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"the name", `{v}>/dev/null; echo "[$v]"`, "[10]\n"},
		{"a target behind it", `{v}>/dev/null 2>/dev/nul${x:=l}; echo "[$v][$x]"`, "[10][l]\n"},
		{"the prefix", `x=1 {w}>/dev/null; echo "[$w][$x]"`, "[10][1]\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// TestAFailedMoveNamesTheWordItExpandedTo — a move whose source is not open
// names the word it was handed, suffix and all, after expansion rather than
// as written (#6627). `x=3`, 3 closed; ksh93u+ 2012-08-01 `-c`, measured
// 2026-10-15: each is `3-: cannot open [Bad file descriptor]`.
func TestAFailedMoveNamesTheWordItExpandedTo(t *testing.T) {
	for _, src := range []string{`true 4<&3-`, `true 4<&$x-`, `true <&$x-`, `y=3-; true 4<&$y`} {
		t.Run(src, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), "x=3; "+src)
			if want := "ksh: 3-: cannot open [Bad file descriptor]\n"; out != want {
				t.Errorf("got %q, want %q", out, want)
			}
		})
	}
}

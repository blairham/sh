// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// With no positional parameters `$@` is set here, so a test that does not
// fire stands for the list — no fields — and a `+` that fires on it is one
// empty field. Measured 2026-10-03 on zsh 5.9.2.
func TestOperatorsOnAnEmptyPositionalList(t *testing.T) {
	src := `set --; for w in "${@-w}" . "${@=w}" . "${@:+w}" . "${@+w}" . "${@:-w}"; do print -rn -- "[$w]"; done; a=(); set -- "${a[@]:+w}"; print -rn -- " $#"`
	const want = "[.][.][][.][w][.][w] 1"
	if out, _ := runZshOnPath(t, t.TempDir(), src); out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

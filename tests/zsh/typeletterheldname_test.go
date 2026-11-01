// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestANumericLetterBesideALiteralOverAHeldName pins #6566: a numeric type
// letter beside an array literal is refused in zsh in one of two sentences,
// and which one is decided by whether the name is already set in the cell the
// declaration lands on. Measured 2026-10-15 on zsh 5.9.2, `-f`, `env -i
// PATH=/usr/bin:/bin`, from `-c`.
func TestANumericLetterBesideALiteralOverAHeldName(t *testing.T) {
	t.Parallel()
	const (
		held   = ": v: can't assign array value to non-array\n"
		unheld = ": v: inconsistent type for assignment\n"
	)
	for _, tc := range []struct{ src, want string }{
		{"typeset -i v=(x)", "zsh:typeset:1" + unheld},
		{"v=1; typeset -i v=(x)", "zsh:typeset:1" + held},
		{"v=1; typeset -F v=(x)", "zsh:typeset:1" + held},
		{"v=1; typeset -E v=(x)", "zsh:typeset:1" + held},
		{"typeset -r v=1; typeset -i v=(x)", "zsh:typeset:1" + held},
		{"v=; typeset -i v=(x)", "zsh:typeset:1" + held},
		{"typeset v; typeset -i v=(x)", "zsh:typeset:1" + held},
		{"v=1; unset v; typeset -i v=(x)", "zsh:typeset:1" + unheld},
		{"v=(a); typeset -i v=(x)", "zsh:typeset:1" + unheld},
		{"typeset -A v; typeset -i v=(x)", "zsh:typeset:1" + unheld},
		{"v=1; typeset -ia v=(x)", "zsh:typeset:1" + held},
		{"v=1; local -i v=(x)", "zsh:local:1" + held},
		{"v=1; export -i v=(x)", "zsh:export:1" + held},
		{"f(){ local -i v=(x); }; v=1; f", "f:local" + unheld},
		{"f(){ local v=1; local -i v=(x); }; f", "f:local" + held},
		{"v=1; f(){ typeset -gi v=(x); }; f", "f:typeset" + held},
		// The numeric words read the literal as `typeset` does.
		{"integer v=(x)", "zsh:integer:1" + unheld},
		{"v=1; integer v=(x)", "zsh:integer:1" + held},
		{"float v=(x)", "zsh:float:1" + unheld},
		{"v=1; float v=(x)", "zsh:float:1" + held},
		// The letter's own sign decides, not the line's last one.
		{"typeset -i +r v=(x)", "zsh:typeset:1" + unheld},
		{"v=1; typeset -i +r v=(x)", "zsh:typeset:1" + held},
		// The controls: no numeric letter, or one taken off, writes the array.
		{"v=1; typeset -Z3 v=(x); typeset -p v", "typeset -aZ3 v=( x )\nafter\n"},
		{"v=1; typeset +i v=(x); typeset -p v", "typeset -a v=( x )\nafter\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), tc.src+"; echo after")
		if out != tc.want {
			t.Errorf("%s\n= %q, want %q", tc.src, out, tc.want)
		}
	}
}

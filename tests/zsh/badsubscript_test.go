// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// The sentence stands alone here: no parameter in front of it for a substring
// range, and no builtin name for `unset`, which words the failure as the
// shell's rather than the builtin's. Measured against zsh 5.9.2 (2026-09-05).
func TestABadSubscriptIsTheBareSentence(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(x y z); echo "[${a[b c]}]"`, "zsh:1: bad math expression: operator expected at `c'\n"},
		{`x=abcdef; echo "[${x:1+:2}]"`, "zsh:1: bad math expression: operand expected at end of string\n"},
	} {
		out, st := runZsh(t, t.TempDir(), c.src)
		if out != c.want || st != 1 {
			t.Errorf("%s = %q (status %d), want %q at 1", c.src, out, st, c.want)
		}
	}
}

// `unset` reports 1 and the next command still runs, so the array is still
// there to be counted.
func TestABadSubscriptToUnsetIsAFailedBuiltin(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `a=(x y z); unset "a[1+]"; echo "st=$? n=${#a[@]}"`)
	want := "zsh:1: bad math expression: operand expected at end of string\nst=1 n=3\n"
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A bare `0x` with no digits after it teaches an integer name base sixteen,
// exactly as `16#` with no digits does. Measured 2026-10-03 on zsh 5.9.2:
// `typeset -i b; b=0x` reads back `16#0` and lists as `typeset -i16 b=0`.
// The `16#` row beside it is the control that was already right, and the
// plain `0` row says the base comes from the prefix rather than the zero.
func TestABareHexPrefixTeachesTheBaseHere(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -i b; b=0x; echo "A [$b]"; typeset -p b
typeset -i c; c=16#; echo "B [$c]"
typeset -i d; d=0; echo "C [$d]"`)
	want := "A [16#0]\ntypeset -i16 b=0\nB [16#0]\nC [0]\n"
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}

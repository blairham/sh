// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A brace range has no length bound (#5140). Measured 2026-10-01: `x=(
// {1..1000000} )` holds a million elements in zsh 5.9.2, bash 5.3.20 and
// ksh93u+ alike, where a bound of ten thousand left `{1..20000}` here as one
// literal word. The ten-thousand row is the control: it was inside the old
// bound and expanded before as well.
func TestABraceRangeHasNoLengthBound(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"x=( {1..10000} ); echo ${#x}", "10000\n"},
		{"x=( {1..20000} ); echo ${#x}", "20000\n"},
		{"x=( {1..1000000} ); echo ${#x}", "1000000\n"},
		{"x=( {20000..1} ); echo ${#x}", "20000\n"},
	} {
		if out, st := runZsh(t, t.TempDir(), c.src+"\n"); out != c.want || st != 0 {
			t.Errorf("%q = %q at %d, want %q", c.src, out, st, c.want)
		}
	}
}

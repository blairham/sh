// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAnArithmeticIncrementOfAnArrayKeepsTheArray: `++`, `--` and a compound
// operator on an array's bare name leave an array holding the value, and on
// an association leave it untouched, where a plain `=` replaces either with a
// number. Measured 2026-10-02 on zsh 5.9.2 (#5369).
func TestAnArithmeticIncrementOfAnArrayKeepsTheArray(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`a=(5); (( a++ )); print ${(t)a} $a ${#a}`, "array 6 1\n"},
		{`a=(); (( ++a )); print ${(t)a} $a`, "array 1\n"},
		{`a=(5); (( a-- )); print ${(t)a} $a`, "array 4\n"},
		{`a=(5); (( a += 2 )); print ${(t)a} $a`, "array 7\n"},
		{`a=(5); let a++; print ${(t)a} $a`, "array 6\n"},
		{`typeset -A h; h[x]=1; (( h++ )); print ${(t)h} ${(kv)h}`, "association x 1\n"},
		// The control: a plain assignment replaces it.
		{`a=(5); (( a = 3 )); print ${(t)a} $a`, "integer 3\n"},
		{`x=5; (( x++ )); print ${(t)x} $x`, "scalar 6\n"},
		{`a=(5); readonly a; (( a++ )); print $a`, "zsh:1: read-only variable: a\n5\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), tc.src)
		if out != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}

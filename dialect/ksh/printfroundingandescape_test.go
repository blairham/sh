// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// Two things this shell's `printf` does that no other column does (#2903,
// #2904). Measured 2026-09-18 against ksh93u+ 2012-08-01 under `LC_ALL=C`
// from a script file, beside bash 5.3.20, bash 3.2.57, zsh 5.9.2, dash
// 0.5.12 and BusyBox ash 1.37.0, which answer every row here the other way.
func TestPrintfRoundsAHalfUpAndDropsAnUndefinedEscape(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// A half of a tenth or more goes away from zero, where the other
		// five take the even neighbor.
		{`printf '%.0f %.0f %.0f\n' 2.5 4.5 -2.5`, "3 5 -3\n"},
		{`printf '%.1f %.2f\n' 0.25 0.125`, "0.3 0.13\n"},
		{`printf '%.0e %.2g\n' 2.5 0.125`, "3e+00 0.13\n"},
		// And below a tenth it goes the other way, which is this shell's own
		// rule rather than a rounding direction: the other five write
		// `0.0938` here.
		{`printf '%.4f\n' 0.09375`, "0.0937\n"},
		// A half the precision reaches no digit of is `0` in every column,
		// this one included.
		{`printf '%.0f\n' 0.5`, "0\n"},
		// An escape the format does not define loses its backslash.
		{`printf '[\q][\z][\8][\-]\n'`, "[q][z][8][-]\n"},
		// The `%b` operand's table is its own and keeps it.
		{`printf '%b\n' '[\q]'`, "[\\q]\n"},
		// A format ending in a backslash writes the backslash, which is
		// unanimous and not this question.
		{`printf 'a\'; printf '\n'`, "a\\\n"},
	} {
		out, st := runKsh(t, t.TempDir(), tc.src)
		if st != 0 || out != tc.want {
			t.Errorf("%s: out %q status %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

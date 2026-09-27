// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// An infinity and a NaN in a float name, which this shell spells in lowercase
// wherever it writes them — in a listing and as a parameter alike, where zsh
// parts the two.
//
// Measured 2026-09-26 on ksh93u+ 2012-08-01 at `/bin/ksh`, from a script
// file. The `1.0/3` row is the control: an ordinary float is unaffected, so
// what was wrong was the round trip through the characters and not the store
// (#4662). See dialect/zsh/floatformat_test.go for the other column.
func TestAFloatNameHoldsAnInfinityAndANaN(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"a NaN", `float f; (( f = 0.0/0.0 )); typeset -p f`, "typeset -l -E f=nan\n"},
		{"an infinity", `float g; (( g = 1.0/0.0 )); typeset -p g`, "typeset -l -E g=inf\n"},
		{"a negative infinity", `float g; (( g = -1.0/0.0 )); typeset -p g`, "typeset -l -E g=-inf\n"},
		{"under the F letter", `typeset -F3 g; (( g = 1.0/0.0 )); typeset -p g`, "typeset -F 3 g=inf\n"},
		{"read as a parameter", `float g; (( g = 1.0/0.0 )); print -- "$g" "${#g}"`, "inf 3\n"},
		{"read as a number", `float g; (( g = 1.0/0.0 )); print -- "$(( g ))" "$(( g + 1 ))"`, "inf inf\n"},
		{"carried into another name", `float g; (( g = 1.0/0.0 )); (( g2 = g )); print -- "$g2"`, "inf\n"},
		{"a declaration's own value", `float g=1.0/0.0; typeset -p g`, "typeset -l -E g=inf\n"},
		// The control.
		{"an ordinary float", `float h; (( h = 1.0/3 )); typeset -p h`, "typeset -l -E h=0.3333333333\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `force_float` makes every operand arithmetic reads a float, and `[#_]`
// groups the digits after a point too. See interp.Runner.forcedFloat and
// interp.groupFraction.
//
// Measured 2026-10-02 on zsh 5.9.2, `-f -c` (#5145).
func TestForceFloatAndFractionGroups(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"constants", "setopt force_float; print $(( 3/4 )) $(( 0x100/0x200 )) $(( 0x30/0x10 )) $(( 3 ))", "0.75 0.5 3. 3.\n"},
		{"names", "setopt force_float; x=5; integer i=7; print $(( x/2 )) $(( i/2 ))", "2.5 3.5\n"},
		{"a name over a name, an element over an element", "setopt force_float; x=5; y=2; a=(5 2); print $(( x/y )) $(( a[1]/a[2] ))", "2.5 2.5\n"},
		{"a declaration is a float", "setopt force_float; (( z = 3 )); typeset -p z", "typeset -F z=3.0000000000\n"},
		{"a bitwise operator is still an integer", "setopt force_float; print $(( 1 << 2 )) $(( ~0 )) $(( 10 & 3 )) $(( 7 % 2 ))", "4 -1 2 1.\n"},
		{"off again", "setopt force_float; unsetopt force_float; print $(( 3/4 ))", "0\n"},
		{"local to a function", "f() { setopt localoptions force_float; print $(( 1/2 )); }; f; print $(( 1/2 ))", "0.5\n0\n"},
		{"grouping a fraction", "print $(( [#_] 1234.5678 )) $(( [#_] (5. ** 10) / 16. )) $(( [#_3] 1234567.5 )) $(( [#_] 1e20 ))", "1_234.567_8 610_351.562_5 1_234_567.5 1e+20\n"},
		{"grouping before an exponent", "print $(( [#_] 1.23456789e30 ))", "1.234_567_89e+30\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, errs := runZshUTF8(t, c.src)
			if out != c.want || errs != "" {
				t.Errorf("%s\n got %q, %q\nwant %q", c.src, out, errs, c.want)
			}
		})
	}
}

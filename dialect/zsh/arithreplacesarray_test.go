// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// An assignment written in arithmetic to a name holding an array replaces it
// with a number declared the way a new name is, and a sequence may not end
// at its comma. See interp.Runner.arithAssignmentReplacesAnArray and
// syntax.Dialect.ArithCommaMayEndTheExpression.
//
// Measured 2026-10-02 on zsh 5.9.2 under `-f -c` (#5145).
func TestAnArithAssignmentReplacesAnArray(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"an empty array", "xarr=(); (( xarr = 3 )); print ${(t)xarr} $xarr", "integer 3\n"},
		{"an array with elements", "a=(1 2); (( a = 3 )); print ${(t)a} $a", "integer 3\n"},
		{"a table", "typeset -A h; (( h = 3 )); print ${(t)h} $h", "integer 3\n"},
		{"a float", "xarr=(); (( xarr = 1.5 )); print ${(t)xarr} $xarr", "float 1.5000000000\n"},
		{"through let and an expansion", "a=(); let a=3; b=(); x=$(( b = 4 )); print ${(t)a} ${(t)b}", "integer integer\n"},
		{"a radix the expression wrote", "xarr=(); (( xarr = 0x1f )); typeset -p xarr", "typeset -i16 xarr=31\n"},
		{"a scalar stays a scalar", "s=abc; (( s = 3 )); print ${(t)s} $s", "scalar 3\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, errs := runZshUTF8(t, c.src)
			if out != c.want || errs != "" {
				t.Errorf("%s\n got %q, %q\nwant %q", c.src, out, errs, c.want)
			}
		})
	}
}

func TestATrailingCommaIsAnOperandThatRanOut(t *testing.T) {
	out, _, errs := runZshUTF8(t, "(( 3, )); print st=$?")
	if out != "st=2\n" || errs != "zsh:1: bad math expression: operand expected at end of string\n" {
		t.Errorf("got %q, %q", out, errs)
	}
}

// An increment is not an assignment of an expression, so it declares nothing:
// measured, `a=(5); (( a++ ))` leaves `${(t)a}` at `array` there. This shell
// leaves a scalar, which is #5369; what is pinned here is the half that
// holds, that no number was declared.
func TestAnArithIncrementDeclaresNothingOverAnArray(t *testing.T) {
	out, _, errs := runZshUTF8(t, "a=(5); (( a++ )); print ${(t)a} $a[1]")
	if out == "integer 6\n" || errs != "" {
		t.Errorf("got %q, %q, want no integer declared", out, errs)
	}
}

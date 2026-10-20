// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// An array letter given a plain word declares a scalar holding it, in a
// function as at the top; a later element write makes it an array, and an
// array literal is the control. Measured 2026-10-03 on ksh93u+ 2012-08-01
// under `-c` (#5630). See interp.Semantics.ArrayLetterWithAWordDeclaresAScalar.
func TestAnArrayLetterWithAWordDeclaresAScalar(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`typeset -a x=/y; typeset -p x`, "x=/y\n"},
		{`function f { typeset -a x=/y; typeset -p x; }; f`, "x=/y\n"},
		{`typeset -a x=/y; x[1]=z; typeset -p x`, "typeset -a x=(/y z)\n"},
		{`typeset -a x=(/y); typeset -p x`, "typeset -a x=(/y)\n"},
		{`typeset -a x="(a b)"; typeset -p x`, "x='(a b)'\n"},
		{`typeset -a x[1]=/y; typeset -p x`, "typeset -a x=([1]=/y)\n"},
	} {
		if out, st := runKshEmptyArray(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d, want %q", c.src, out, st, c.want)
		}
	}
}

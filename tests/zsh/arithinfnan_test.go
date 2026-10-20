// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `inf` and `nan`, in any case, are the floating constants here, before any
// parameter of that name, and are no place to store anything. See
// interp.Semantics.ArithInfAndNaNAreConstants.
//
// Measured 2026-10-02 on zsh 5.9.2, `-f -c` (#5145).
func TestInfAndNaNAreConstants(t *testing.T) {
	for _, c := range []struct{ name, src, want, errs string }{
		{"before a parameter", "in=1 info=2 Infinity=3 Inf=4; print $(( in )) $(( info )) $(( Infinity )) $(( $Inf )) $(( inf )) $(( INF )) $(( Inf )) $(( iNF ))", "1 2 3 4 Inf Inf Inf Inf\n", ""},
		{"nan", "nan=5; print $(( nan )) $(( NaN ))", "NaN NaN\n", ""},
		{"in an expression", "print $(( inf + 1 )) $(( -inf )) $(( inf - inf )) $(( nan == nan )) $(( inf > 1 ))", "Inf -Inf NaN 0 1\n", ""},
		{"from a value", "x=inf; print $(( x ))", "Inf\n", ""},
		{"a declaration is a float", "(( n = inf )); typeset -p n", "typeset -F n=inf\n", ""},
		{"an assignment is refused", "(( NaN = 1 )); print st=$?", "st=2\n", "zsh:1: bad math expression: lvalue required\n"},
		{"an increment is refused", "(( Inf++ )); print st=$?", "st=2\n", "zsh:1: bad math expression: lvalue required\n"},
		{"brackets after one", "(( Inf[1] )); print st=$?", "st=2\n", "zsh:1: bad base syntax\n"},
		{"brackets after one, stored to", "(( Inf[1] = 2 )); print st=$?; (( Inf[1]++ )); print st=$?", "st=2\nst=2\n", "zsh:1: bad base syntax\nzsh:1: bad base syntax\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, errs := runZshUTF8(t, c.src)
			if out != c.want || errs != c.errs {
				t.Errorf("%s\n got %q, %q\nwant %q, %q", c.src, out, errs, c.want, c.errs)
			}
		})
	}
}

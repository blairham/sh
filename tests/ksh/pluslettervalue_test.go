// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A declaration whose letters are all plus-signed stores its value through the
// letters the name still carries, and then takes them off. Measured 2026-10-03
// on ksh93u+ 2012-08-01 under `-c` (#5663). See
// interp.Semantics.PlusLetterComesOffAfterTheValueLands.
func TestAPlusLetterComesOffAfterTheValueLands(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`typeset -l s=A; typeset +l s=B; typeset -p s`, "s=b\n"},
		{`typeset -u s=a; typeset +u s=b; typeset -p s`, "s=B\n"},
		{`typeset -lx s=A; typeset +l s=B; typeset -p s`, "typeset -x s=b\n"},
		{`typeset -lt s=A; typeset +l s=B; typeset -p s`, "typeset -t s=b\n"},
		{`typeset -l s=A; typeset +lu s=Bc; typeset -p s`, "s=bc\n"},
		{`typeset -lx s=A; typeset +l +x s=Bc; typeset -p s`, "s=bc\n"},
		{`typeset -l -L4 s=A; typeset +L s=B; typeset -p s`, "typeset -l s='b   '\n"},
		{`typeset -l -a s=(A); typeset +l s=Bc; typeset -p s`, "typeset -a s=(bc)\n"},
		{`typeset -l s=A; typeset +l s=B t=C; typeset -p s t`, "s=b\nt=C\n"},
		// The controls: the letter taken off with no value, and a keyword
		// function's fresh local, which has nothing to fold with.
		{`typeset -l s=A; typeset +l s; s=B; typeset -p s`, "s=B\n"},
		{`typeset -l s=A; function f { typeset +l s=B; typeset -p s; }; f; typeset -p s`, "s=B\ntypeset -l s=a\n"},
	} {
		if out, st := runKshEmptyArray(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d, want %q", c.src, out, st, c.want)
		}
	}
}

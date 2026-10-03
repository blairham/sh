// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A `-p` listing whose first option word is plus-signed writes each row
// without its value. The key is that first word's sign and not the sign on
// `p`. Measured 2026-10-03 on ksh93u+ 2012-08-01 under `-c` (#5642). See
// interp.Semantics.PlusSignedPrintListsNoValues.
func TestAPlusSignedPrintListsNoValues(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`x=/y; typeset +p x`, "x\n"},
		{`typeset -i n=1; typeset +p n`, "typeset -i n\n"},
		{`typeset -a a=(1 2); typeset +p a`, "typeset -a a\n"},
		{`typeset -A m=([k]=v); typeset +p m`, "typeset -A m\n"},
		{`typeset -rx r=1; typeset +p r`, "typeset -x -r r\n"},
		{`typeset -Z3 z=1; typeset +p z`, "typeset -Z 3 -R 3 z\n"},
		{`typeset -n ref=x; typeset +p ref`, "typeset -n ref\n"},
		{`function f { typeset x=1; typeset +p x; }; f`, "x\n"},
		// The first word's sign decides, whichever letter it carries.
		{`typeset -i n=5; typeset +i -p n`, "typeset -i n\n"},
		{`typeset -i n=5; typeset +x +p n; typeset -p n`, "typeset -i n\ntypeset -i n=5\n"},
		{`typeset -i n=5; typeset -i +p n`, "typeset -i n=5\n"},
		{`typeset -i n=5; typeset -p +p n`, "typeset -i n=5\n"},
		// The whole table, and a filtered one.
		{`typeset -i qq=5; zz=7; typeset +p | while read -r l; do case $l in *qq*|*zz*) print -r -- "$l";; esac; done`, "typeset -i qq\nzz\n"},
		{`typeset -i qq=5; typeset +p -i | while read -r l; do case $l in *qq*) print -r -- "$l";; esac; done`, "typeset -i qq\n"},
		// An operand the listing assigns first is listed after it lands,
		// still without its value.
		{`typeset +p s=5; typeset -p s`, "s\ns=5\n"},
	} {
		if out, st := runKshEmptyArray(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d, want %q", c.src, out, st, c.want)
		}
	}
}

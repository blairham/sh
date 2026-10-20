// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A declaration operand's `=` is looked for after its subscript (#6781).
// Measured 2026-10-16 on ksh93u+ 2012-08-01 from a script file.
func TestADeclarationOperandsSubscriptMayHoldAnEquals(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"typeset -A m; typeset 'm[a=b]=w'; v='m[c=d]=x'; typeset \"$v\"; typeset -p m\n", "typeset -A m=([a=b]=w [c=d]=x)\n"},
		{"typeset -A m; typeset 'm[b[1]=2]=v'; typeset -p m\n", "typeset -A m=(['b[1]=2']=v)\n"},
	} {
		if out, _ := answersRun(t, c.src); out != c.want {
			t.Errorf("%q\n got %q\nwant %q", c.src, out, c.want)
		}
	}
}

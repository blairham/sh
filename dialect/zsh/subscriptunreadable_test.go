// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A byte the arithmetic cannot read ends a subscript's expression: what is in
// front of it is the subscript, and nothing in front of it is the operand
// expected at it. Measured 2026-10-03 on zsh 5.9.2 under `-f -c` with
// `a=(x y z)` (#5567). See
// interp.Semantics.SubscriptExpressionStopsAtAnUnreadableByte.
func TestAnUnreadableByteEndsASubscriptsExpression(t *testing.T) {
	const arr = "a=(x y z); "
	for _, c := range []struct{ src, want string }{
		{`i=2; echo ${a[2@]} ${a[i@]} ${a[1'x']}`, "y y x\n"},
		{`echo ${a['2']}`, "zsh:1: bad math expression: operand expected at `'2''\n"},
		{`echo ${a[1+@]}`, "zsh:1: bad math expression: operand expected at `@'\n"},
		{`i="'2"; echo ${a[$i]}`, "zsh:1: bad math expression: operand expected at `'2'\n"},
		{`unset 'a['\''1'\'']'`, "zsh:1: bad math expression: operand expected at `'1''\n"},
		// The control: outside a subscript the byte is refused by name.
		{`echo $(( '1' ))`, "zsh:1: bad math expression: illegal character: '\n"},
	} {
		if out, _ := runZsh(t, t.TempDir(), arr+c.src); out != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
		}
	}
}

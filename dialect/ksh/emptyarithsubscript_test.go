// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// Brackets with nothing between them hold an expression that is empty, and an
// empty expression is zero — so the operand is *element zero* and not the
// number zero. Measured against ksh93u+ (2026-09-10): the stream stays clean
// and the status stays 0, which is the only one of the three answers the panel
// gives that says nothing at all.
//
// The array row is the one that carries the distinction. A probe on an unset
// name would read zero under every answer the panel has.
func TestAnEmptyArithmeticSubscriptIsTheEmptyExpression(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`a=(5 6 7); echo $(( a[] )); echo after`, "5\nafter\n"},
		{`a=(5 6 7); echo $(( a[0] )); echo after`, "5\nafter\n"},
		{`m=9; echo $(( m[] )); echo after`, "9\nafter\n"},
		{`echo $(( nodecl[] )); echo after`, "0\nafter\n"},
		// And an operator writing through the same brackets steps element
		// zero rather than being refused.
		{`a=(5 6 7); (( a[]++ )); echo "[${a[*]}]"`, "[6 6 7]\n"},
	} {
		if out, st := runKsh(t, t.TempDir(), tc.src); out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

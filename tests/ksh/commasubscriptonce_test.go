// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A comma in a subscript is the arithmetic operator here, and the subscript is
// evaluated once, before the array is read: an assignment in it is seen by the
// read it belongs to, and a step in it steps once, in an expansion, an
// assignment's subscript and `unset` alike.
//
// Measured 2026-10-05 with ksh93u+, bash 5.3.20 and bash 3.2.57, alike in every row. The comma used to
// be worked out as a range *and* as the operator, to see whether the two
// readings differed, so every step and assignment in it ran twice, and the
// array was read before either: the rows read `x`, `y`, `1`, `z 2`, ` 4`,
// `2 5` and `2 0 1 2 3 4 6`. The last row has nothing in it to run (#6120).
func TestACommaSubscriptIsEvaluatedOnceBeforeTheRead(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(x y); echo "${a[a[0]=7,0]}"`, "7\n"},
		{`a=(x y z); echo "${a[a[1]=Q,1]}"`, "0\n"},
		{`a=(x y z); echo "${#a[a[0]=1234,0]}"`, "4\n"},
		{`a=(x y z); i=0; echo "${a[i++,i]}" $i`, "y 1\n"},
		{`a=(x y z); i=0; echo "${a[i++,i++]}" $i`, "y 2\n"},
		{`i=0; a[i++,5]=x; echo $i ${!a[@]}`, "1 5\n"},
		{`i=0; a=(p q r s t u v); unset "a[i++,5]"; echo $i ${!a[@]}`, "1 0 1 2 3 4 6\n"},
		{`a=(x y); echo "${a[1,1]}"`, "y\n"},
	} {
		if out, st := runKsh(t, t.TempDir(), c.src); out != c.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", c.src, out, st, c.want)
		}
	}
}

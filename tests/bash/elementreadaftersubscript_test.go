// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// An element read sees what its own subscript wrote: the subscript is
// evaluated first and the array read after it.
//
// Measured 2026-10-05 with bash 5.3.20, bash 3.2.57 and ksh93u+, all three
// alike. The array used to be read whole before the subscript ran, so the
// first two rows read the element as it had been (`x`, `y`); the holey third
// row was already right, because a gap sent the read to the store. The last
// row is the control that has nothing to see: a subscript that only moves a
// counter (#6099).
func TestAnElementReadSeesWhatItsSubscriptWrote(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(x y); echo "${a[(a[0]=3)*0]}"`, "3\n"},
		{`a=(x y); echo "${a[(a[1]=7)>0]}"`, "7\n"},
		{`a=(x); a[5]=y; echo "${a[(a[5]=4)+1]}"`, "4\n"},
		{`a=(x y); i=0; echo "${a[i++]}${a[i++]}"`, "xy\n"},
	} {
		if out, st := runBash(t, t.TempDir(), c.src); out != c.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", c.src, out, st, c.want)
		}
	}
}

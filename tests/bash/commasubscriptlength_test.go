// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A comma subscript is one arithmetic subscript here, so its length past the
// first element is refused the way `${#a[-4]}` is: the subscript as written,
// brackets and no name, and the rest of the line given up. Measured
// 2026-10-03 on bash 5.3.20 with `a=(1 2 3 4 5)`:
//
//	echo "${#a[-1,-8]}"; echo same   [-1,-8]: bad array subscript, no `same`
//	echo "${#a[-8,-8]}"              [-8,-8]: bad array subscript
//	echo "${#a[0,2]}"                1 — the comma's last operand, read
//
// The read of the same element is the control, naming the array.
func TestTheLengthOfACommaSubscriptReachedPastIsRefused(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`a=(1 2 3 4 5); echo "${#a[-1,-8]}"; echo same` + "\necho next", "[-1,-8]: bad array subscript\nnext\n"},
		{`a=(1 2 3 4 5); echo "${#a[-8,-8]}"; echo same` + "\necho next", "[-8,-8]: bad array subscript\nnext\n"},
		{`a=(1 2 3 4 5); echo "${#a[0,2]}"`, "1\n"},
		{`a=(1 2 3 4 5); echo "[${a[-8,-8]}]"`, "a: bad array subscript\n[]\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.HasSuffix(out, tc.want) {
				t.Errorf("= %q, want it to end %q", out, tc.want)
			}
		})
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A subscript whose text expanded to nothing is the expression that is zero
// here, so it names element zero and nothing is said about it.
//
// Measured 2026-09-11 on bash 5.3.15, each row a `-c` of its own. It is here
// as well as in the zsh package because the axis is only visible as a
// disagreement: one of these two files passing alone would say nothing about
// which reading this dialect has.
func TestASubscriptThatExpandedToNothingIsElementZero(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`a=(5 6 7); w=; echo "[${a[$w]}]"`, "[5]"},
		{`a=(5 6 7); echo "[${a[ ]}]"`, "[5]"},
		{`a=(5 6 7); w="  "; echo "[${a[$w]}]"`, "[5]"},
		{`s=hi; w=; echo "[${s[$w]}]"`, "[hi]"},
		{`a=(5 6 7); w=; echo "[${#a[$w]}]"`, "[1]"},
		{`a=(5 6 7); w=; a[$w]=z; echo "[${a[0]}]"`, "[z]"},
		{`w=; echo "[${nosucharr[$w]}]"`, "[]"},
	} {
		out, status := answersRun(t, tc.src+`; echo after`)
		if got := strings.TrimSpace(out); got != tc.want+"\nafter" || status != 0 {
			t.Errorf("%s = %q (status %d), want %q then after, at 0", tc.src, got, status, tc.want)
		}
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A `*` subscript inside an expression is an ordinary subscript here and not
// a slice, so an association answers zero for the key it has not got.
//
// Measured 2026-09-11 on bash 5.3.15: `typeset -A m; m[k]=9; $(( m[*] ))` is
// `0` where the expansion `"${m[*]}"` beside it is `9`. Here as well as in
// the zsh package because the axis is only visible as a disagreement.
func TestAWholeArraySubscriptInAnExpressionIsNotASlice(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`typeset -A m; m[k]=9; echo $(( m[*] ))`, "0"},
		{`typeset -A m; m[k]=9; echo $(( m[@] ))`, "0"},
		{`typeset -A m; m[k]=9; echo "${m[*]}"`, "9"},
	} {
		out, status := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want || status != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, got, status, tc.want)
		}
	}
}

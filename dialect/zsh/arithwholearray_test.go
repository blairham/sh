// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A `*` or `@` subscript inside an expression is the slice here — the same
// one `${m[*]}` takes — and the joined text is then read as an expression.
//
// Measured 2026-09-11 on zsh 5.9.2, each row a `-c` of its own.
func TestAWholeArraySubscriptInAnExpressionIsTheSlice(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`typeset -A m; m[k]=9; echo $(( m[*] ))`, "9"},
		{`typeset -A m; m[k]=9; echo $(( m[@] ))`, "9"},
		{`a=(3); echo $(( a[*] + 1 ))`, "4"},
		{`a=(); echo $(( a[*] ))`, "0"},
		{`echo $(( nosuch[*] ))`, "0"},
		{`s=7; echo $(( s[*] ))`, "7"},
		{`a=(3 4); IFS=; echo $(( a[*] ))`, "34"},
		{`a=(3 4); IFS=; echo $(( a[@] ))`, "34"},
		// The expansion route agreed all along, and is here as the control
		// the arithmetic row has to be read against.
		{`typeset -A m; m[k]=9; echo "${m[*]}"`, "9"},
	} {
		out, status := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want || status != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, got, status, tc.want)
		}
	}
}

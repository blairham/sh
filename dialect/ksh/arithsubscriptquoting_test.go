// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// ksh93 takes the quoting off an indexed subscript written inside `$(( ))`,
// as it does for one in `${a[…]}` — where the line's read would have taken
// `'1'` for a character's code. Measured 2026-10-03 on ksh93u+ 2012-08-01
// under `-c` (#5594). The last rows are the controls: a quoted character
// outside a subscript is still its code, and a backslash inside a double
// quotation stays.
func TestAQuotedSubscriptInArithmeticLosesItsQuoting(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(1 2 3); echo $(( a['1'] + 1 )) $(( a[ '1' ] )) $(( a['1+1'] )) $(( a[\1] ))`, "3 2 3 2\n"},
		{`a=(1 2 3); (( a['1'] = 9 )); echo ${a[@]}`, "1 9 3\n"},
		{`echo $(( '2' + 1 ))`, "51\n"},
	} {
		if out, st := runKshEmptyArray(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d, want %q", c.src, out, st, c.want)
		}
	}
	out, _ := runKshEmptyArray(t, `a=(1 2 3); echo $(( a["\1"] ))`)
	if !strings.Contains(out, `\1: arithmetic syntax error`) {
		t.Errorf("got %q, want the backslash refused", out)
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// A colon where a group's `)` belongs is an unbalanced parenthesis here, and
// other text in that place is not. Measured 2026-10-04 on ksh93u+ 2012-08-01
// under `-c` (#5722). See interp.Diagnostics.ArithColonInAGroup.
func TestAColonInAnArithmeticGroupIsAnUnbalancedParenthesis(t *testing.T) {
	for _, c := range []struct{ src, errs string }{
		{"echo $(( (1:2) ))", "ksh:  (1:2) : unbalanced parenthesis\n"},
		{"echo $(( (1 :2) ))", "ksh:  (1 :2) : unbalanced parenthesis\n"},
		{"a=(1 2); echo ${a[(rn:2:)*a]}", "ksh: (rn:2:)*a: unbalanced parenthesis\n"},
		{"echo $(( (1 2) ))", "ksh:  (1 2) : arithmetic syntax error\n"},
		{"echo $(( (1?2) ))", "ksh:  (1?2) : ':' expected for '?' operator\n"},
	} {
		_, errs, _ := runKshArgs(t, "-c", c.src)
		if errs != c.errs {
			t.Errorf("%s:\n got %q\nwant %q", c.src, errs, c.errs)
		}
	}
}

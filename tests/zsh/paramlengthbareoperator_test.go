// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestALengthPrefixOverABareOperatorIsTheParameter is this shell's side of
// the split #4166 measured.
//
// Behind a `${#`, an operator with **no operand at all** keeps the `#` as the
// parameter `$#` here, where the other five columns scan the operator as the
// name the length is over, find none, and refuse the whole expansion. With an
// operand every column agrees, so the disagreement is the empty word rather
// than the operator.
//
// Measured on zsh 5.9.2 with `set -- p q`, so `$#` is 2: `${#+}` is empty —
// the alternate fires, and it is empty — and `${#=}` is `2`, the assignment
// never firing on a parameter that is set. See
// syntax.Dialect.ParamLengthBareOperatorIsTheParameter for the panel this is
// the dissenting column of.
func TestALengthPrefixOverABareOperatorIsTheParameter(t *testing.T) {
	out, st := answersRun(t,
		"set -- p q\n"+
			"echo \"[${#+}]\"\necho \"[${#=}]\"\n"+
			"echo \"[${#+w}]\"\necho \"[${#=w}]\"\n")
	want := "[]\n[2]\n[w]\n[2]\n"
	if out != want || st != 0 {
		t.Errorf("got\n%s\n(status %d)\nwant\n%s", out, st, want)
	}
}

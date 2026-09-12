// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// This shell's `test` and `[` read their comparison operands as arithmetic,
// the way `[[ ]]` reads its own (#1626), and a condition operand's leading
// zeros go in front of a radix prefix as well (#1627).
//
// Measured 2026-09-12 against ksh93u+ 2012-08-01, `-c` under `env -i`.

func TestSingleBracketReadsItsComparisonOperandsAsArithmetic(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`n=5; [ n -eq 5 ] && echo yes || echo no`, "yes"},
		{`[ 1+1 -eq 2 ] && echo yes || echo no`, "yes"},
		{`n=5; [ n+1 -eq 6 ] && echo yes || echo no`, "yes"},
		{`[ "" -eq 0 ] && echo yes || echo no`, "yes"},
		{`[ 16#10 -eq 16 ] && echo yes || echo no`, "yes"},
		// The expression language and not a lookup: an assignment written in
		// an operand lands, and `n` is nine afterwards.
		{`n=5; [ "n=9" -eq 9 ] && echo "$n" || echo no`, "9"},
		// `test` under its own name, which is the same builtin.
		{`n=5; test n -eq 5 && echo yes || echo no`, "yes"},
		// The zeros come off the front of the operand, so a leading `0x` is
		// no prefix: `0x10` is the name `x10`.
		{`[ 0x10 -eq 0 ] && echo yes || echo no`, "yes"},
		{`x10=7; [ 0x10 -eq 7 ] && echo yes || echo no`, "yes"},
		{`[ 010 -eq 10 ] && echo yes || echo no`, "yes"},
		{`[ 0777 -eq 777 ] && echo yes || echo no`, "yes"},
		{`[ 010#5 -eq 5 ] && echo yes || echo no`, "yes"},
		{`[ 0b101 -eq 0 ] && echo yes || echo no`, "yes"},
		// And nothing in front of the zeros means nothing to take off, which
		// is what says this is the rewrite and not a reader with no hex in
		// it: `1+0x10` is seventeen.
		{`[ 1+0x10 -eq 17 ] && echo yes || echo no`, "yes"},
		{`[ -0x10 -eq -16 ] && echo yes || echo no`, "yes"},
	} {
		out, _ := answersRun(t, tc.src)
		if strings.TrimSpace(out) != tc.want {
			t.Errorf("%s: got %q, want %q", tc.src, out, tc.want)
		}
	}
}

// An operand the arithmetic cannot read is loud, blames the builtin by the
// name it was called by, ends at 1 rather than the not-an-expression 2, and
// leaves the script running — where the same words inside `[[ ]]` abandon the
// input in this shell.
func TestASingleBracketArithmeticRefusalIsNotFatal(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`[ 1x1 -eq 0 ]; echo "st=$?"; echo after`, "sh: [: 1x1: arithmetic syntax error\nst=1\nafter\n"},
		{`test 1x1 -eq 0; echo "st=$?"`, "sh: test: 1x1: arithmetic syntax error\nst=1\n"},
		{`[ 3/0 -eq 0 ]; echo "st=$?"`, "sh: [: 3/0: divide by zero\nst=1\n"},
		{`x=hello; [ x -eq 0 ]; echo "st=$?"`, "sh: [: hello: parameter not set\nst=1\n"},
		// The double-bracket construct words it without the builtin's name
		// and takes the script with it, which is the contrast.
		{`[[ 1x1 -eq 0 ]]; echo "st=$?"; echo after`, "sh: 1x1: arithmetic syntax error\n"},
	} {
		out, _ := answersRun(t, tc.src)
		if out != tc.want {
			t.Errorf("%s: got %q, want %q", tc.src, out, tc.want)
		}
	}
}

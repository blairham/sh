// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// A comparison operand that is not an integer is refused in one of two
// sentences, and which one depends on where the reading of a number stopped:
// an operand with no digits at its front, or with more than fit, is `out of
// range`, and one that began with digits and went on with something else is
// `bad number`.
//
// Measured 2026-10-04 in the digest-pinned alpine image, BusyBox v1.37.0,
// over `[ "$v" -eq 1 ]` — see Diagnostics.TestIntegerTrailingJunk (#5723).
func TestAComparisonOperandIsRefusedByWhereItStopped(t *testing.T) {
	for _, tc := range []struct{ operand, want string }{
		{"abc", "abc: out of range"},
		{"n=9", "n=9: out of range"},
		{"-", "-: out of range"},
		{"9999999999999999999x", "9999999999999999999x: out of range"},
		{"1x1", "1x1: bad number"},
		{"1+1", "1+1: bad number"},
		{"0x10", "0x10: bad number"},
		{"-5x", "-5x: bad number"},
		{" 1/0 ", " 1/0 : bad number"},
	} {
		t.Run(tc.operand, func(t *testing.T) {
			out, st := run(t, "[ '"+tc.operand+"' -eq 1 ]\necho \"st=$?\"\n")
			if !strings.Contains(out, tc.want) || !strings.Contains(out, "st=2") {
				t.Errorf("[ %q -eq 1 ] said %q (exit %d), want %q and st=2", tc.operand, out, st, tc.want)
			}
		})
	}
	// And a numeral with blanks around it is taken, which is what puts the
	// line after the blanks rather than at the first byte that is not a digit.
	if out, _ := run(t, "[ ' 5 ' -eq 5 ]; echo \"st=$?\"\n"); !strings.Contains(out, "st=0") {
		t.Errorf("[ ' 5 ' -eq 5 ] said %q, want st=0", out)
	}
}

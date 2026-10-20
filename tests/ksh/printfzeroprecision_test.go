// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// C ignores the `0` flag outright where a precision is written on an integer
// conversion, and six of the seven panel columns comply. This one does not:
// the precision is applied and the field is then filled with `0` rather than
// with blanks.
//
// Measured 2026-09-17 against ksh93u+ 2012-08-01 under LC_ALL=C, at a nonzero
// value as well as at the nought #3024 found it beside — the nought alone
// cannot tell a precision that was ignored from one that produced no digits
// (#3067).
func TestTheZeroFlagSurvivesAPrecision(t *testing.T) {
	for _, tc := range []struct{ spec, arg, want string }{
		{"%+05.0d", "7", "+0007"},
		{"%#05.0o", "7", "00007"},
		{"%08.0d", "7", "00000007"},
		{"%05.2d", "7", "00007"},
		{"%08.3d", "7", "00000007"},
		{"%+08.3d", "7", "+0000007"},
		{"%#08.3o", "7", "00000007"},
		{"%08.3x", "7", "00000007"},
		{"%08.3X", "7", "00000007"},
		{"%+05.0d", "0", "+0000"},
		{"%08.0d", "0", "00000000"},
		{"%05.2d", "0", "00000"},
		// The controls, and each one takes a simpler reading off the table.
		// Without the flag, with `-`, and with no width the columns agree;
		// and a precision wider than the width is unanimous, which says the
		// precision is applied here and not ignored.
		{"%8.3d", "7", "     007"},
		{"%-08.3d", "7", "007     "},
		{"%.3d", "7", "007"},
		{"%08.10d", "7", "0000000007"},
	} {
		out, st := answersRun(t, "printf '[%"+"s]' \"$(printf '"+tc.spec+"' "+tc.arg+")\"")
		if st != 0 {
			t.Errorf("%s of %s: status %d: %q", tc.spec, tc.arg, st, out)
			continue
		}
		if want := "[" + tc.want + "]"; !strings.Contains(out, want) {
			t.Errorf("%s of %s: got %q, want it to contain %q", tc.spec, tc.arg, out, want)
		}
	}
}

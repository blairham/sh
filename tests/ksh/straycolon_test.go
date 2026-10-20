// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A stray `:` is the one construct this shell writes back to front (#2224).
//
// Every other math complaint it makes is `<expression>: <reason>`; for a
// colon with no `?` in front of it the offending byte comes first, then the
// reason, then the whole expression after a ` - `. Measured 2026-09-12
// against ksh93u+ 2012-08-01 over every printable byte in `$(( 1 <byte> ))`:
// `:` is the only one that takes this order. The expression is quoted back as
// the construct held it, so these have no surrounding blanks where the
// measurements written with `$(( 1 : ))` do.
func TestAStrayColonReversesTheOrder(t *testing.T) {
	for _, tc := range []struct{ expr, want string }{
		{"1 :", ":: invalid character in expression - 1 :"},
		{"1 : 2", ":: invalid character in expression - 1 : 2"},
		// Wherever the colon stands, a complete conditional in front of it
		// included.
		{"1 ? 2 : 3 : 4", ":: invalid character in expression - 1 ? 2 : 3 : 4"},
		// And the controls, in the ordinary order: a leftover that is not a
		// colon, a byte the reader refuses outright, and a colon that wants
		// a *value* rather than standing left over.
		{"1 2", "1 2: arithmetic syntax error"},
		{"1 @", "1 @: arithmetic syntax error"},
		{": 1", ": 1: arithmetic syntax error"},
	} {
		if got := arithLine(t, tc.expr); got != arithLoc+tc.want+"\n" {
			t.Errorf("$((%s)): got %q, want %q", tc.expr, got, arithLoc+tc.want+"\n")
		}
	}
}

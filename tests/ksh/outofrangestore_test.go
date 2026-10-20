// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A store through `read`'s operand whose subscript lands out of range is a
// failed `read` here, worded with the builtin in front, and the line goes on;
// through `typeset` it ends the script, as the statement does. `read` ended
// the script (#6835).
//
// Measured 2026-10-16 on ksh93u+ 2012-08-01 from a file, byte for byte apart
// from the location's file name.
func TestAnOutOfRangeStoreThroughReadFailsTheBuiltin(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"a=(1)\nread 'a[-5]' <<< x; echo st=$?\necho next\n", "sh[2]: read: a: subscript out of range\nst=1\nnext\n"},
		{"a=(1)\ntypeset a[-5]=x; echo st=$?\necho next\n", "sh: line 2: a: subscript out of range\n"},
	} {
		if out, _ := answersRun(t, c.src); out != c.want {
			t.Errorf("%q\n got %q\nwant %q", c.src, out, c.want)
		}
	}
}

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

// A literal under the indexed letter whose element comes to a position before
// the first keeps the elements it placed in front of it, the way one whose
// subscript would not evaluate already did. It kept the name's old value
// (#6852). Measured 2026-10-30 on ksh93u+ 2012-08-01 from a file, read by an
// exit trap: `subscript out of range`, then `typeset -a b=(z)`.
func TestAnOutOfRangeElementKeepsWhatTheLiteralPlaced(t *testing.T) {
	src := "b=(7)\ntrap 'typeset -p b' EXIT\ntypeset -a b=([0]=z [-5]=x [2]=q)\n"
	want := "sh: line 3: b: subscript out of range\ntypeset -a b=(z)\n"
	if out, _ := answersRun(t, src); out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

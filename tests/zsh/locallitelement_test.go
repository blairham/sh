// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A literal written through a subscript as a declaration's operand may not
// make the array a local of the call, as a scalar written through one may
// not: the refusal ends the script. Over a local the call already made it
// splices, and at the top level there is nothing to make (#6828).
//
// Measured 2026-10-16 on zsh 5.9.2 `-f` from a file, byte for byte apart from
// the location.
func TestALiteralThroughASubscriptCannotCreateALocal(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ src, want string }{
		{"f(){ local b[1]=(p q); echo st=$?; }; f; echo end", "f:local: b[1]: can't create local array elements\n"},
		{"f(){ typeset b[1]=(p q); echo st=$?; }; f; echo end", "f:typeset: b[1]: can't create local array elements\n"},
		{"a=(1 2); f(){ local a[1]=(p q); }; f; echo end", "f:local: a[1]: can't create local array elements\n"},
		{"f(){ local -a b; local b[1]=(p q); echo st=$?; typeset -p b; }; f", "st=0\ntypeset -a b=( p q )\n"},
		{"typeset b[1]=(p q); typeset -p b", "typeset -a b=( p q )\n"},
	} {
		if out, _ := answersRun(t, c.src); out != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
		}
	}
}

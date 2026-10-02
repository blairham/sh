// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// An element store the arithmetic refuses fails the expression here, as
// `1/0` does: the construct answers with its own status and fatality. See
// interp.Semantics.ArithStoreRefusalIsAnError.
//
// Measured 2026-10-02 on zsh 5.9.2 under `-f -c` (#5145).
func TestAnArithStoreRefusalIsAnError(t *testing.T) {
	const sentence = "newarray: assignment to invalid subscript range"
	for _, c := range []struct{ name, src, want string }{
		{"(( )) leaves 2 and carries on", "(( newarray[0] = 1 )); print st=$?", "st=2\n"},
		{"an unset index is the same", "(( newarray[unsetvar] = 1 )); print st=$?", "st=2\n"},
		{"let leaves 1", "let 'newarray[0] = 1'; print st=$?", "st=1\n"},
		{"inside a function too", "f() { (( newarray[0] = 1 )); print in=$?; }; f; print out=$?", "in=2\nout=0\n"},
		{"an expansion ends the shell", "x=$(( newarray[0] = 1 )); print st=$?", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, errs := runZshUTF8(t, c.src)
			if out != c.want || strings.Count(errs, sentence) != 1 || strings.Contains(errs, "let:") {
				t.Errorf("%s\n got %q, %q\nwant %q and the sentence once, located as the shell", c.src, out, errs, c.want)
			}
		})
	}
}

// And a store outside arithmetic after one inside it is refused as before,
// written and fatal: measured, `(( newarray[0] = 1 )); newarray[0]=2; print
// no` writes the sentence twice and exits 1 without the `no`.
func TestAStoreAfterAnArithStoreRefusalIsRefusedAsBefore(t *testing.T) {
	out, st, errs := runZshUTF8(t, "(( newarray[0] = 1 )); newarray[0]=2; print no")
	if out != "" || st != 1 || strings.Count(errs, "newarray: assignment to invalid subscript range") != 2 {
		t.Errorf("got %q at %d, %q, want nothing at 1 and the sentence twice", out, st, errs)
	}
}

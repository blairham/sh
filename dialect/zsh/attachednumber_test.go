// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A number attached to its letter takes the rest of the word, and a rest that
// is not a number is refused in the letter's own words, at 1, with nothing
// declared. Measured 2026-10-03 on zsh 5.9.2 under `-fc` (#5685). See
// interp.Semantics.AttachedNumberEndsAtTheFirstNonDigit.
func TestAnAttachedNumberTakesTheRestOfTheWord(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`typeset -Z3x s=7; echo st=$?; typeset -p s`, "zsh:typeset:1: bad width value: 3x\nst=1\nzsh:typeset:1: no such variable: s\n"},
		{`typeset -i16x s=255; echo st=$?`, "zsh:typeset:1: bad base value: 16x\nst=1\n"},
		{`typeset -F2x s=1; echo st=$?`, "zsh:typeset:1: bad precision value: 2x\nst=1\n"},
		{`typeset -R3l s=AB; echo st=$?`, "zsh:typeset:1: bad width value: 3l\nst=1\n"},
		{`typeset -Z3x2 s=7; echo st=$?`, "zsh:typeset:1: bad width value: 3x2\nst=1\n"},
	} {
		if out, _ := runZsh(t, t.TempDir(), c.src); out != c.want {
			t.Errorf("%s\n got %q, want %q", c.src, out, c.want)
		}
	}
}

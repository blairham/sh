// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A control character where an operator belonged is a byte the arithmetic
// reader refuses outright, named visibly; after an operator it is still the
// operand that was missing.
//
// Measured 2026-10-16 on zsh 5.9.2, from a file (#6723).
func TestAControlCharacterInArithmeticIsAnIllegalCharacter(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, want string }{
		{`[[ $'a\x01' -eq 1 ]]`, "zsh:1: bad math expression: illegal character: ^A\n"},
		{`let $'1 \x7f'`, "zsh:1: bad math expression: illegal character: ^?\n"},
		{`let $'\r'`, "zsh:1: bad math expression: illegal character: ^M\n"},
		{`let $'1+\x01'`, "zsh:1: bad math expression: operand expected at `^A'\n"},
		{`let $'(1\x1b)'`, "zsh:1: bad math expression: ')' expected\n"},
	} {
		_, _, errs := runZshUTF8(t, tc.src)
		if errs != tc.want {
			t.Errorf("%s: got %q, want %q", tc.src, errs, tc.want)
		}
	}
}

// A `%` and a code that names no escape in that shell draws nothing, at
// status 0, in every reader; an escape it has and this one lacks is still
// refused by name.
//
// Measured 2026-10-16 on zsh 5.9.2 `-f`, from a file (#6724).
func TestAnEscapeTheShellLacksIsDropped(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, out, errs string }{
		{`print -P 'x%Qy'; print st=$?`, "xy\nst=0\n", ""},
		{`print -P 'x%J%,%;y'; print st=$?`, "xy\nst=0\n", ""},
		{`print -P $'x%\x01y'; print st=$?`, "xy\nst=0\n", ""},
		{`print -rn -- ${(%):-$'x%\x01y'}; print st=$?`, "xyst=0\n", ""},
		{`print -P 'x%yy'; print st=$?`, "st=1\n", "zsh:print:1: the %y prompt escape is not implemented\n"},
	} {
		out, _, errs := runZshUTF8(t, tc.src)
		if out != tc.out || errs != tc.errs {
			t.Errorf("%s: got %q and %q, want %q and %q", tc.src, out, errs, tc.out, tc.errs)
		}
	}
}

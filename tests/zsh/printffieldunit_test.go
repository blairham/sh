// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A string conversion's field is counted in characters here where the locale
// has them, and the `l` modifier changes nothing. Measured 2026-09-16 on zsh
// 5.9.2 from a script file under `env -i PATH=/usr/bin:/bin`, with `LC_ALL` as
// each row sets it (#2298). See Semantics.PrintfFieldCountsCharacters.
func TestAPrintfFieldIsCharactersWhereTheLocaleHasThem(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a precision", `LC_ALL=en_US.UTF-8; printf '[%.2s]' αβγ`, "[αβ]"},
		{"a width", `LC_ALL=en_US.UTF-8; printf '[%7s]' αβγ`, "[    αβγ]"},
		{"%b", `LC_ALL=en_US.UTF-8; printf '[%.2b]' αβγ`, "[αβ]"},
		{"%q is bytes", `LC_ALL=en_US.UTF-8; printf '[%.2q]' αβγ`, "[α]"},
		{"the l is ignored", `LC_ALL=en_US.UTF-8; printf '[%.2ls]' αβγ`, "[αβ]"},
		{"%lc is %c's byte", `LC_ALL=en_US.UTF-8; printf '[%lc]' αβγ`, "[\xce]"},
		{"the C locale", `LC_ALL=C; printf '[%.2s|%7s]' αβγ αβγ`, "[α| αβγ]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

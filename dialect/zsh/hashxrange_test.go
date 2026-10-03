// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestACharacterCodeTheLocaleHasNoneForIsReportedUnderX pins that `(#X)`
// refuses a code at or above 0x80 in the C or POSIX locale, or none at all,
// while `(#)` alone writes the byte and `nomultibyte` takes any (#5151, a
// chunk of D04parameter.ztst). Measured 2026-10-03 on zsh 5.9.2 under `-f`.
func TestACharacterCodeTheLocaleHasNoneForIsReportedUnderX(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`LC_ALL=C; (: ${(#X):-0x80}); echo st=$?`, "zsh:1: character not in range\nst=1\n"},
		{`LANG=POSIX; x=128; (: ${(#X)x}); echo st=$?`, "zsh:1: character not in range\nst=1\n"},
		// The controls.
		{`LC_ALL=C; print -r -- ${(#X):-0x41} ${#${(#):-0x80}} ${#${(#X):-0x7f}}`, "A 1 1\n"},
		{`unset LC_ALL LC_CTYPE LANG; (: ${(#X):-0x80}); echo st=$?`, "zsh:1: character not in range\nst=1\n"},
		{`LC_ALL=C; setopt nomultibyte; print -r -- ${#${(#X):-0x80}}`, "1\n"},
		{`LC_ALL=en_US.UTF-8; print -r -- ${(#X):-0xe9}`, "é\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

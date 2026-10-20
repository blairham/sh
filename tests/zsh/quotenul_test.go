// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAQuotedNulIsBackslashZero pins that the quoting flags spell a NUL as
// `\0`, and as `\000` only before an octal digit. Measured 2026-10-02 on zsh
// 5.9.2 under `-f` (#5336).
func TestAQuotedNulIsBackslashZero(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`x=$'a\0b'; print -r -- ${(q)x} ${(qqqq)x}`, `a$'\0'b $'a\0b'` + "\n"},
		{`x=$'\0'1; print -r -- ${(q)x} ${(qqqq)x}`, `$'\000'1 $'\0001'` + "\n"},
		{`x=$'\0'8; print -r -- ${(q)x}`, `$'\0'8` + "\n"},
		{`x=$'\1'; print -r -- ${(q)x}`, `$'\001'` + "\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"
)

// A `\u` escape naming a code point the locale cannot hold leaves the escape
// **standing**, normalized, and the command carries on.
//
// Measured 2026-09-11 against bash 5.3.15, bytes read with `od`. In a UTF-8
// locale this shell was already right — that is where a person's terminal is,
// and where the panel agrees — so the C locale is the whole of the difference,
// and it is where every conformance run happens to sit (#1851).
//
// The assertions are bytes, because the wrong answers are all
// plausible-looking text: the encoded character, the source escape echoed back
// unchanged, and nothing at all are three strings a Contains check on the
// letter would not tell apart.
func TestAUnicodeEscapeOutsideTheLocaleStandsAsWritten(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the escape stands", `LC_ALL=C; echo -e 'a\u00e9Z'`, `a\u00E9Z`},
		// Normalized rather than echoed, and all three parts of that are
		// measured: the digits are padded to four, upper-cased, and the
		// letter is chosen by the value rather than taken from the input.
		{"padded to four digits", `LC_ALL=C; echo -e 'a\ue9Z'`, `a\u00E9Z`},
		{"the letter follows the value", `LC_ALL=C; echo -e 'a\U000000e9Z'`, `a\u00E9Z`},
		{"eight digits once four will not do", `LC_ALL=C; echo -e 'a\U1F600Z'`, `a\U0001F600Z`},
		// The command carries on, which is the whole difference from the
		// other answer to this axis.
		{"and the script carries on", `LC_ALL=C; echo -e 'a\u00e9Z'; echo AFTER`, "a" + `\u00E9` + "Z\nAFTER"},

		// A UTF-8 locale writes the character, which is where this shell was
		// already right and where the panel agrees.
		{"a UTF-8 locale writes it", `LC_ALL=en_US.UTF-8; echo -e 'a\u00e9Z'`, "a\u00e9Z"},
		// ASCII is representable in every encoding, so the boundary is the
		// code point rather than the escape.
		{"ASCII is in range even in C", `LC_ALL=C; echo -e 'a\u0041Z'`, "aAZ"},
		// Reading the variables the way POSIX ranks them, which is the same
		// reader `${#s}` uses: a non-empty LC_ALL outranks LANG.
		{"LC_ALL outranks LANG", `LANG=en_US.UTF-8; LC_ALL=C; echo -e 'a\u00e9Z'`, `a\u00E9Z`},
		{"and LANG decides on its own", `LANG=C; echo -e 'a\u00e9Z'`, `a\u00E9Z`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), tc.src)
			if got := trimNewline(out); got != tc.want || st != 0 {
				t.Errorf("%s = %q status %d, want %q status 0", tc.src, got, st, tc.want)
			}
		})
	}
}

func trimNewline(s string) string {
	for len(s) > 0 && s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}
	return s
}

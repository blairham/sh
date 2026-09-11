// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// The other sites that read a `@u` escape leave it **standing**, normalized,
// exactly as `echo -e` does — and the command carries on.
//
// Measured 2026-09-11 on bash 5.3.15 under `LC_ALL=C`. Each of these wrote the
// UTF-8 here before (#2021): right in a UTF-8 locale, where a person's
// terminal is, and wrong in the C locale a conformance run sits in.
func TestEverySiteLeavesACodePointOutsideTheLocaleStanding(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a printf format", `LC_ALL=C; printf 'a\u00e9Z'; echo '|AFTER'`, `a\u00E9Z|AFTER`},
		{"a printf %b argument", `LC_ALL=C; printf '%b' 'a\u00e9Z'; echo '|AFTER'`, `a\u00E9Z|AFTER`},
		{"a $'...' word", `LC_ALL=C; printf '%s' $'a\u00e9Z'; echo '|AFTER'`, `a\u00E9Z|AFTER`},
		// Normalized rather than echoed, at these sites too: padded to four
		// digits, upper-cased, and the letter chosen by the value.
		{"padded and upper-cased", `LC_ALL=C; printf '%s' $'a\ue9Z'`, `a\u00E9Z`},
		{"eight digits once four will not do", `LC_ALL=C; printf '%s' $'a\U1F600Z'`, `a\U0001F600Z`},
		// A UTF-8 locale writes the character at every one of them.
		{"a UTF-8 locale writes it", `LC_ALL=en_US.UTF-8; printf '%s' $'a\u00e9Z'`, "a\u00e9Z"},
		{"and so does a format", `LC_ALL=en_US.UTF-8; printf 'a\u00e9Z'`, "a\u00e9Z"},
		// ASCII is representable in every encoding, so the boundary is the
		// code point rather than the escape.
		{"ASCII is in range even in C", `LC_ALL=C; printf '%s' $'a\u0041Z'`, "aAZ"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), tc.src)
			if got := trimNewline(out); got != tc.want || st != 0 {
				t.Errorf("%s = %q status %d, want %q status 0", tc.src, got, st, tc.want)
			}
		})
	}
}

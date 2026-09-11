// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A locale nothing names is the C locale here, on every operator that reads
// one.
//
// Measured 2026-09-11 under `env -i`, with no LC_ALL, LC_CTYPE or LANG set
// anywhere: zsh 5.9.2 answers these exactly as it answers them under
// `LC_ALL=C`, where bash 5.3.15 answers all three as it does in a UTF-8
// locale — 5, `CAFÉ` and the encoded character (#2020).
//
// Three operators rather than one, because the same state decides all three
// and this shell used to answer the length this way and the other two the
// other.
func TestAnUnsetLocaleIsTheCLocale(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// 6 and not 5: bytes, as under an explicit C locale.
		{"a length counts bytes", `s=héllo; echo "${#s}"`, "6"},
		// Case mapping is narrowed to ASCII, which this shell did not do.
		{"case mapping is ASCII alone", `s=café; echo "${(U)s}"`, "CAFé"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if got := trimTrailingNewlines(out); got != tc.want || st != 0 {
				t.Errorf("%s = %q status %d, want %q status 0", tc.src, got, st, tc.want)
			}
		})
	}
}

// And a `\u` escape naming a character the encoding has no room for is
// refused, the way it is refused under an explicit C locale: the complaint,
// the text before the escape, nothing after it, and the next command left
// unrun.
func TestAnUnsetLocaleRefusesACodePointOutsideIt(t *testing.T) {
	const src = `echo 'a\u00e9Z'; echo AFTER`
	const want = "zsh:1: character not in range\na\n"
	out, st := runZsh(t, t.TempDir(), src)
	if out != want || st != 0 {
		t.Errorf("%s = %q status %d, want %q status 0", src, out, st, want)
	}
}

func trimTrailingNewlines(s string) string {
	for len(s) > 0 && s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}
	return s
}

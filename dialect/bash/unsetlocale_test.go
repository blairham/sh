// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A locale nothing names is UTF-8-capable here, which is this shell alone in
// the panel.
//
// Measured 2026-09-11 under `env -i`, with no LC_ALL, LC_CTYPE or LANG set
// anywhere — which is what a cron job and a container have, and what a
// person's terminal never has. bash 5.3.15 answers every one of these the way
// it answers them in a UTF-8 locale, while ksh93u+, zsh 5.9.2 and dash answer
// them the way they answer under `LC_ALL=C` (#2020).
//
// Three operators rather than one, because the same state decides all three
// and this shell used to answer two of them one way and the third the other.
func TestAnUnsetLocaleIsUnicodeAware(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// 5 and not 6: characters, as in a UTF-8 locale.
		{"a length counts characters", `s=héllo; echo "${#s}"`, "5"},
		{"and a substring is positioned in them", `s=héllo; echo "${s:1:1}"`, "é"},
		// Case mapping reaches beyond ASCII, which this shell already did.
		{"case mapping is not narrowed to ASCII", `s=café; echo "${s^^}"`, "CAFÉ"},
		// And the escape is written rather than left standing, since the
		// encoding has room for the character.
		{"a code point is written", `echo -e 'a\u00e9Z'`, "a\u00e9Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), tc.src)
			if got := trimNewline(out); got != tc.want || st != 0 {
				t.Errorf("%s = %q status %d, want %q status 0", tc.src, got, st, tc.want)
			}
		})
	}
}

// And an explicit C locale is still the C locale, which is what says the
// answer above is about *nothing being named* rather than about this shell
// ignoring the variables.
func TestAnExplicitCLocaleStillNarrows(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a length counts bytes", `LC_ALL=C; s=héllo; echo "${#s}"`, "6"},
		{"case mapping is ASCII alone", `LC_ALL=C; s=café; echo "${s^^}"`, "CAFé"},
		{"and the escape stands", `LC_ALL=C; echo -e 'a\u00e9Z'`, `a\u00E9Z`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), tc.src)
			if got := trimNewline(out); got != tc.want || st != 0 {
				t.Errorf("%s = %q status %d, want %q status 0", tc.src, got, st, tc.want)
			}
		})
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A locale nothing names is the C locale here, which is the standard's answer
// and this preset keeps it.
//
// Measured 2026-09-11 under `env -i`, with no LC_ALL, LC_CTYPE or LANG set
// anywhere: ksh93u+ answers 6 for `s=héllo; echo ${#s}`, the same answer it
// gives under `LC_ALL=C`, where bash 5.3.15 answers 5 to the same line —
// which is the split #2020 records.
func TestAnUnsetLocaleIsTheCLocale(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a length counts bytes", `s=héllo; echo "${#s}"`, "6"},
		{"and so is a substring's position", `s=héllo; echo "${s:1:2}"`, "\xc3\xa9"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), tc.src)
			if got := strings.TrimRight(out, "\n"); got != tc.want || st != 0 {
				t.Errorf("%s = %q status %d, want %q status 0", tc.src, got, st, tc.want)
			}
		})
	}
}

// And a UTF-8 locale is still read, which is what says the answer above is
// about nothing being named rather than about this shell counting bytes
// always.
func TestANamedUtf8LocaleCountsCharacters(t *testing.T) {
	const src = `LC_ALL=en_US.UTF-8; s=héllo; echo "${#s}"`
	out, st := runKsh(t, t.TempDir(), src)
	if got := strings.TrimRight(out, "\n"); got != "5" || st != 0 {
		t.Errorf("%s = %q status %d, want %q status 0", src, got, st, "5")
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A code point the locale's encoding cannot hold is written all the same here:
// this shell consults no locale for the escape, which is the third answer
// beside leaving the escape standing and refusing it.
//
// Measured 2026-09-11 on ksh93u+ under `LC_ALL=C`, bytes read with `od`. It is
// reachable at two sites and no more, because this shell reads no `@u` escape
// in `echo`, in `print` or in a `%b` argument — and the rows that show that are
// here too, since an answer at a site the shell never reaches would be a claim
// about nothing (#2021).
func TestACodePointOutsideTheLocaleIsWrittenAnyway(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a printf format", `LC_ALL=C; printf 'a\u00e9Z'`, "a\u00e9Z"},
		{"a $'...' word", `LC_ALL=C; printf '%s' $'a\u00e9Z'`, "a\u00e9Z"},
		// The same in a UTF-8 locale, which is what "no locale is consulted"
		// means: the two rows agree where the other two shells' do not.
		{"and the same in a UTF-8 locale", `LC_ALL=en_US.UTF-8; printf 'a\u00e9Z'`, "a\u00e9Z"},
		// The three sites this shell does not read the escape at, where the
		// ten characters stand whatever the locale says.
		{"no such escape in echo", `LC_ALL=C; echo 'a\u00e9Z'`, `a\u00e9Z`},
		{"nor in print", `LC_ALL=C; print -- 'a\u00e9Z'`, `a\u00e9Z`},
		{"nor in a %b argument", `LC_ALL=C; printf '%b' 'a\u00e9Z'`, `a\u00e9Z`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), tc.src)
			if got := strings.TrimRight(out, "\n"); got != tc.want || st != 0 {
				t.Errorf("%s = %q status %d, want %q status 0", tc.src, got, st, tc.want)
			}
		})
	}
}

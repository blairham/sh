// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// What `${!prefix@}` comes to when the names carrying the prefix are held in
// a compound table. Measured 2026-09-16 on ksh93u+ 2012-08-01 under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, from a script file with stdin closed
// (#2298).
//
// This column is the other side of Semantics.PrefixListingNamesADeclaredOnly-
// Compound: a table the letters merely declared is a name here, where bash
// 5.3 leaves it out. The rest of the listing agrees between the two, which is
// what makes the declared-only pair the axis rather than the whole listing.

func runKshPrefixListing(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st, err := preset.CombinedWithPrelude(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

func TestAPrefixListingOverTheCompoundTables(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a keyed table", `typeset -A zqm=([k]=v); echo "[${!zq@}]"`, "[zqm]\n"},
		{"an indexed array", `typeset -a zqa=(x); echo "[${!zq@}]"`, "[zqa]\n"},
		{"a subscripted assignment", `zqa[3]=x; echo "[${!zq@}]"`, "[zqa]\n"},
		{"both, sorted", `typeset -A zqm=([k]=v); zqa[3]=x; echo "[${!zq@}]"`, "[zqa zqm]\n"},
		// The axis, answered the other way: the letters alone are enough to
		// put the name in the listing here.
		{"declared only, keyed", `typeset -A zqm; echo "[${!zq@}]"`, "[zqm]\n"},
		{"declared only, indexed", `typeset -a zqa; echo "[${!zq@}]"`, "[zqa]\n"},
		{"assigned empty, keyed", `typeset -A zqm=(); echo "[${!zq@}]"`, "[zqm]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKshPrefixListing(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

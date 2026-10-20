// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"testing"

	"github.com/blairham/sh/driver"
)

// What `${!prefix@}` comes to when the names carrying the prefix are held in
// a compound table. Measured 2026-09-16 on bash 5.3.20 under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, from a script file with stdin closed
// (#2298).
//
// The keyed table is the one that had no other home: an array has a scalar
// cell beside it and a table has none, so a listing built from the scalar
// cells alone knew the name everywhere else and not here. The consequence is
// not an empty listing — `declare -p ${!m@}` with nothing to substitute is
// `declare -p` with no operands, which prints the whole shell.

func runPrefixListing(t *testing.T, src string) (string, string, int) {
	t.Helper()
	var out, errs bytes.Buffer
	code := driver.MainArgs(bashShell(&out, &errs), []string{"bash", "-c", src})
	return out.String(), errs.String(), code
}

func TestAPrefixListingOverTheCompoundTables(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a keyed table", `declare -A zqm=([k]=v); echo "[${!zq@}]"`, "[zqm]\n"},
		{"an indexed array", `declare -a zqa=(x); echo "[${!zq@}]"`, "[zqa]\n"},
		{"a subscripted assignment", `zqa[3]=x; echo "[${!zq@}]"`, "[zqa]\n"},
		{"both, sorted", `declare -A zqm=([k]=v); zqa[3]=x; echo "[${!zq@}]"`, "[zqa zqm]\n"},
		// The declared-only pair, which is this column's own answer and the
		// one ksh93 parts from: the letters alone do not put the name in the
		// listing, and an assignment of an empty compound does.
		{"declared only, keyed", `declare -A zqm; echo "[${!zq@}]"`, "[]\n"},
		{"declared only, indexed", `declare -a zqa; echo "[${!zq@}]"`, "[]\n"},
		{"assigned empty, keyed", `declare -A zqm=(); echo "[${!zq@}]"`, "[zqm]\n"},
		{"assigned empty, indexed", `declare -a zqa=(); echo "[${!zq@}]"`, "[zqa]\n"},
		// It is the assignment that moves the name and not the emptiness.
		{"written then emptied", `declare -A zqm; zqm[k]=v; unset "zqm[k]"; echo "[${!zq@}]"`, "[zqm]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := runPrefixListing(t, tc.src)
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", tc.src, out, errs, st, tc.want)
			}
		})
	}
}

// The use the listing is put to, and the reason the empty answer was not a
// cosmetic one: a substitution that comes to nothing leaves the builtin with
// no operands, and `declare -p` with no operands prints every name the shell
// holds. One row, not a shell.
func TestAPrefixListingStandingAsAnOperand(t *testing.T) {
	out, errs, st := runPrefixListing(t, `declare -A zqm=([k]=v); declare -p ${!zq@}`)
	if want := "declare -A zqm=([k]=\"v\" )\n"; out != want || errs != "" || st != 0 {
		t.Errorf("declare -p ${!zq@} = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

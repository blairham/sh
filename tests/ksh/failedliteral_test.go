// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// An array literal that fails partway through stops expanding there. A plain
// one stores nothing; one whose first element names its subscript leaves the
// pairs it had placed, started over as a table — or as an indexed array under
// a declaration's `-a` — and an append keeps them only onto a table.
// Measured 2026-10-15 on ksh93u+ 2012-08-01 from a file, this script, byte for
// byte apart from the location in front of each complaint. The elements after
// the failure ran here, and every subscripted row kept the old value (#6606,
// #6608).
func TestAFailedSubscriptedLiteralKeepsWhatItStored(t *testing.T) {
	src := "row() { ( trap 'typeset -p a m 2>/dev/null' EXIT; eval \"$1\" ); }\n" +
		"row 'a=(1 2); a=(5 $(echo SIDE >&2) $((1/0)) $(echo AFTER >&2))'\n" +
		"row 'a=(1 2); typeset a=(5 $(echo SIDE >&2) $((1/0)) $(echo AFTER >&2))'\n" +
		"row 'a=(1 2); typeset -a a=([2]=x [0]=$((1/0)))'\n" +
		"row 'a=(1 2); a=([0]=x [5]=y [1]=$((1/0)) [2]=$(echo AFTER >&2))'\n" +
		"row 'a=(1 2); a=([$((1/0))]=v)'\n" +
		"row 'typeset -a a=(1 2); a=([0]=x [1]=$((1/0)))'\n" +
		"row 'a=(1 2); typeset a=([0]=x [1]=$((1/0)))'\n" +
		"row 'a=(1 2); a+=([5]=x [6]=$((1/0)))'\n" +
		"row 'a=(1 2); a=(x [3]=$((1/0)))'\n" +
		"row 'typeset -A m; m=([k]=old); m=([j]=new [k]=$((1/0)))'\n" +
		"row 'typeset -A m; m=([k]=old); m+=([k]=new [i]=$((1/0)))'\n" +
		"row 'typeset -A m; m=([k]=old); m+=(j new i $((1/0)))'\n"
	want := "SIDE\n" +
		"ERR\n" +
		"typeset -a a=(1 2)\n" +
		"SIDE\n" +
		"ERR\n" +
		"typeset -a a=(1 2)\n" +
		"ERR\n" +
		"typeset -a a=([2]=x)\n" +
		"ERR\n" +
		"typeset -A a=([0]=x [5]=y)\n" +
		"ERR\n" +
		"typeset -A a=()\n" +
		"ERR\n" +
		"typeset -A a=([0]=x)\n" +
		"ERR\n" +
		"typeset -A a=([0]=x)\n" +
		"ERR\n" +
		"typeset -a a=(1 2)\n" +
		"ERR\n" +
		"typeset -a a=(1 2)\n" +
		"ERR\n" +
		"typeset -A m=([j]=new)\n" +
		"ERR\n" +
		"typeset -A m=([k]=new)\n" +
		"ERR\n" +
		"typeset -A m=([k]=old)\n"
	out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatal(err)
	}
	if got := elementOrderNormalized(out); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

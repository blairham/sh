// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A subscript in a read-modify-write is evaluated once, and a table's key is
// a key and not an expression.
//
// Measured 2026-10-02 on zsh 5.9.2, `-f` (#5145).
func TestAReadModifyWriteSubscriptIsEvaluatedOnce(t *testing.T) {
	out, _, errs := runZshUTF8(t, "array=(1); x=0; (( array[++x]++ )); print $x $#array $array; a=(5 6 7); i=1; (( a[i++] += 10 )); print $i $a; c=(1 2); j=0; (( c[j+=2]-- )); print $j $c")
	if want := "1 1 2\n2 15 6 7\n2 1 1\n"; out != want || errs != "" {
		t.Errorf("got %q, %q, want %q", out, errs, want)
	}
	out, _, errs = runZshUTF8(t, "typeset -A h; (( h[ab]++ )); (( h[ab] += 5 )); print ${(kv)h}")
	if want := "ab 6\n"; out != want || errs != "" {
		t.Errorf("table: got %q, %q, want %q", out, errs, want)
	}
}

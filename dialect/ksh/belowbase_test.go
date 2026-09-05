// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// The array alone is named here, not the subscript, and the script ends at 1.
// Measured against ksh93u+ 2012-08-01 (2026-09-05).
func TestANegativeSubscriptPastTheStartIsRefused(t *testing.T) {
	for _, src := range []string{
		`a=(p q); a[-3]=x; echo ok`,
		`a=(p q); a[-3]+=Q; echo ok`,
		`a[-1]=x; echo ok`,
		`x=1; a[x-2]=v; echo ok`,
	} {
		out, st := runKsh(t, t.TempDir(), src)
		want := "ksh: a: subscript out of range\n"
		if out != want || st != 1 {
			t.Errorf("%s = %q (status %d), want %q at 1", src, out, st, want)
		}
	}
}

// From `unset` the array is named as it is for an assignment, with the
// builtin in front of the sentence — which it does not put there for the
// assignment. The script runs on. Measured against ksh93u+ 2012-08-01
// (2026-09-05).
func TestUnsetPastTheStartIsRefused(t *testing.T) {
	for _, src := range []string{
		`a=(x y z); unset "a[-4]"; echo "st=$? n=${#a[@]}"`,
		`a=(x y z); unset "a[x-9]"; echo "st=$? n=${#a[@]}"`,
	} {
		out, st := runKsh(t, t.TempDir(), src)
		want := "ksh: unset: a: subscript out of range\nst=1 n=3\n"
		if out != want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", src, out, st, want)
		}
	}
}

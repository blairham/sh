// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// The first element is 1 here, so `a[0]` is below it: the assignment is
// refused and the script ends at 1. Measured against zsh 5.9.2 (2026-09-05).
func TestASubscriptBelowTheFirstElementIsRefused(t *testing.T) {
	for _, src := range []string{
		`a=(p q); a[0]=x; echo ok`,
		`a=(p q); a[0]+=Q; echo ok`,
		`a[0]=x; echo ok`,
	} {
		out, st := runZsh(t, t.TempDir(), src)
		want := "zsh:1: a: assignment to invalid subscript range\n"
		if out != want || st != 1 {
			t.Errorf("%s = %q (status %d), want %q at 1", src, out, st, want)
		}
	}
}

// Through a literal the sentence is a different one, and it names the
// subscript rather than the array.
func TestASubscriptBelowTheFirstElementInALiteral(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `a=([0]=p); echo ok`)
	want := "zsh:1: bad subscript for direct array assignment: 0\n"
	if out != want || st != 1 {
		t.Errorf("got %q (status %d), want %q at 1", out, st, want)
	}
}

// A negative subscript is not below the first element here — it counts back
// from the last — and one that runs past the start places an element in front
// of every other, however far past it went.
func TestANegativeSubscriptPastTheStartInserts(t *testing.T) {
	for _, src := range []string{
		`a=(p q); a[-3]=x; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`,
		`a=(p q); a[-5]=x; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`,
	} {
		out, st := runZsh(t, t.TempDir(), src)
		if out != "[x][p][q] n=3\n" || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", src, out, st, "[x][p][q] n=3\n")
		}
	}
}

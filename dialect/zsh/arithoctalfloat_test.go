// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A leading zero under octalzeroes does not make a float or a base octal,
// and a subscript in a read-modify-write is evaluated once.
//
// Measured 2026-10-02 on zsh 5.9.2, `-f`, each with the option set on a line
// before the expression so that the line is read with it on (#5145).
func TestALeadingZeroDoesNotMakeAFloatOrABaseOctal(t *testing.T) {
	out, _, errs := runZshUTF8(t, "setopt octalzeroes\nprint $(( 09.5 )) $(( 07.5 )) $(( 01e2 )) $(( 08#77 )) $(( 010 )) $(( 016 ))")
	if want := "9.5 7.5 100. 63 8 14\n"; out != want || errs != "" {
		t.Errorf("got %q, %q, want %q", out, errs, want)
	}
}

func TestAReadModifyWriteSubscriptIsEvaluatedOnce(t *testing.T) {
	out, _, errs := runZshUTF8(t, "array=(1); x=0; (( array[++x]++ )); print $x $#array $array; a=(5 6 7); i=1; (( a[i++] += 10 )); print $i $a; c=(1 2); j=0; (( c[j+=2]-- )); print $j $c")
	if want := "1 1 2\n2 15 6 7\n2 1 1\n"; out != want || errs != "" {
		t.Errorf("got %q, %q, want %q", out, errs, want)
	}
}

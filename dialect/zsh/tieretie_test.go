// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestTyingAPairAgainJoinsItsElementsWithTheNewSeparator pins what `typeset
// -T` does over a pair that is already tied: the elements are joined with the
// separator the line names and split on it again, and a scalar value written
// on that line keeps the old separator instead. Measured 2026-10-02 on zsh
// 5.9.2 under `-f`, every row.
func TestTyingAPairAgainJoinsItsElementsWithTheNewSeparator(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"letters and a separator over a tied pair",
			"typeset -T VAR var=(a b a b); typeset -UuT VAR var +; print $VAR; typeset -p VAR var",
			"A+B\ntypeset -uUT VAR var=( a b ) +\ntypeset -auUT VAR var=( a b ) +\n",
		},
		{
			"a separator alone",
			"typeset -T V v=(x y); typeset -T V v -; print $V ${#v}",
			"x-y 2\n",
		},
		{
			"the scalar was assigned last",
			"typeset -T V v; V=a:b; typeset -T V v +; print $V ${#v}",
			"a+b 2\n",
		},
		{
			"an element holding the new separator splits",
			"typeset -T V v +; v=(a:b c); typeset -T V v; print $V ${#v}",
			"a:b:c 3\n",
		},
		{
			"an empty pair comes back with one empty element",
			`typeset -T V v; typeset -T V v +; print "[$V]" ${#v}`,
			"[] 1\n",
		},
		{
			"a scalar value keeps the old separator",
			"typeset -T V=a:b v; typeset -T V=c:d v +; typeset -p V; V=e+f; print ${#v}",
			"typeset -T V v=( c d )\n1\n",
		},
		{
			"an array value takes the new one",
			"typeset -T W w=(a b); typeset -T W w=(c d) +; print $W",
			"c+d\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, status := runZsh(t, t.TempDir(), tc.src)
			if got != tc.want || status != 0 {
				t.Errorf("%s\n got %q status %d\nwant %q", tc.src, got, status, tc.want)
			}
		})
	}
}

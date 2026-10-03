// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestARcExpandLookupOfOneValueIsTheEmptyString pins that under
// `rc_expand_param` a lookup naming one value that found none keeps the word
// around it, where a lookup naming a list removes it (#5151, a chunk of
// D04parameter.ztst). Measured 2026-10-02 on zsh 5.9.2
// (`/opt/homebrew/bin/zsh -f`, `LC_ALL=C`).
func TestARcExpandLookupOfOneValueIsTheEmptyString(t *testing.T) {
	const setup = "setopt rcexpandparam; a=(p q); typeset -A h; h=(X x)\n"
	for _, tc := range []struct{ src, want string }{
		{`print -r -- S n=$a[5] v=$a[(r)Y] k=$h[(i)y] r=$h[(r)Y] m=$h[zz] E`, "S n= v= k= r= m= E\n"},
		{`print -r -- S z=$a[(R)Y] w=$h[(I)y] x=$h[(K)zz] E`, "S E\n"},
		{`print -l S $a[5] $h[(i)y] E`, "S\nE\n"},
		// The controls: a lookup that found something, and a count.
		{`print -r -- S m=$h[X] k=$a[(i)y] l=${#a[5]} E`, "S m=x k=3 l=0 E\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
